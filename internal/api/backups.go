package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/files"
	"github.com/fomk/389-ds-ui/internal/providers"
)

type fileListOutput struct{ Body []files.Info }
type fileNamePath struct {
	Name string `path:"name"`
}
type createBackupInput struct {
	Body struct {
		Name string `json:"name,omitempty" doc:"Directory name; default backup_<timestamp>"`
	}
}
type exportTaskInput struct {
	Name string `path:"name" doc:"Backend"`
	Body struct {
		File        string `json:"file,omitempty" doc:"File name in the LDIF directory; default <backend>_<timestamp>.ldif"`
		Replication bool   `json:"replication,omitempty" doc:"Include replication state (for initialising replicas)"`
	}
}
type importTaskInput struct {
	Name string `path:"name" doc:"Backend"`
	Body struct {
		File string `json:"file" minLength:"1" doc:"File name in the LDIF directory. The backend's current content is replaced."`
	}
}

// serverDirs reads the backup and LDIF directories from cn=config.
func serverDirs(ctx context.Context, d Deps) (bak, ldif string, err error) {
	err = d.LDAP.Do(ctx, func(c *ldap.Conn) (err error) { bak, ldif, err = ds.ServerDirs(c); return })
	return
}

// store opens a server directory on the shared volume (file access needed).
func store(ctx context.Context, d Deps, backups bool) (files.Store, error) {
	if d.Files == nil {
		return files.Store{}, providers.ErrNotSupported
	}
	bak, ldif, err := serverDirs(ctx, d)
	if err != nil {
		return files.Store{}, err
	}
	dir := ldif
	if backups {
		dir = bak
	}
	return files.Open(d.Files.Root, dir)
}

func stamp() string { return time.Now().UTC().Format("20060102_150405") }

func registerBackups(api huma.API, d Deps) {
	tags := []string{"backups"}
	startTask := func(ctx context.Context, typ, target string, attrs map[string][]string) (*taskOutput, error) {
		t, err := d.Tasks.Start(ctx, typ, target, attrs)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &taskOutput{Status: http.StatusAccepted, Body: t}, nil
	}

	huma.Register(api, huma.Operation{OperationID: "listBackups", Method: http.MethodGet, Path: apiPrefix + "/backups",
		Summary: "Backups in nsslapd-bakdir (needs file access)", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*fileListOutput, error) {
			s, err := store(ctx, d, true)
			if err != nil {
				return nil, toHTTPError(err)
			}
			list, err := s.List(true)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &fileListOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "createBackup", Method: http.MethodPost, Path: apiPrefix + "/backups",
		Summary: "Back up all databases (online task)", Tags: tags, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *createBackupInput) (*taskOutput, error) {
			name := in.Body.Name
			if name == "" {
				name = "backup_" + stamp()
			}
			if !files.ValidName(name) {
				return nil, toHTTPError(fmt.Errorf("%w: %q", files.ErrInvalidName, name))
			}
			bak, _, err := serverDirs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return startTask(ctx, ds.TaskBackup, name, map[string][]string{
				"nsArchiveDir": {filepath.Join(bak, name)}, "nsDatabaseType": {"ldbm database"}})
		})

	huma.Register(api, huma.Operation{OperationID: "restoreBackup", Method: http.MethodPost, Path: apiPrefix + "/backups/{name}/restore",
		Summary: "Restore all databases from a backup (online task; backends go offline briefly)", Tags: tags, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *fileNamePath) (*taskOutput, error) {
			if !files.ValidName(in.Name) {
				return nil, toHTTPError(fmt.Errorf("%w: %q", files.ErrInvalidName, in.Name))
			}
			bak, _, err := serverDirs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			if d.Files != nil { // check it exists when we can see the directory
				if s, err := files.Open(d.Files.Root, bak); err == nil {
					if _, err := s.Stat(in.Name); err != nil {
						return nil, toHTTPError(err)
					}
				}
			}
			return startTask(ctx, ds.TaskRestore, in.Name, map[string][]string{
				"nsArchiveDir": {filepath.Join(bak, in.Name)}, "nsDatabaseType": {"ldbm database"}})
		})

	huma.Register(api, huma.Operation{OperationID: "downloadBackup", Method: http.MethodGet, Path: apiPrefix + "/backups/{name}/download",
		Summary: "Download a backup as .tar.gz", Tags: tags,
		Responses: map[string]*huma.Response{"200": {Description: "tar.gz archive", Content: map[string]*huma.MediaType{"application/gzip": {}}}}},
		func(ctx context.Context, in *fileNamePath) (*huma.StreamResponse, error) {
			s, err := store(ctx, d, true)
			if err != nil {
				return nil, toHTTPError(err)
			}
			if _, err := s.Stat(in.Name); err != nil {
				return nil, toHTTPError(err)
			}
			return &huma.StreamResponse{Body: func(hctx huma.Context) {
				hctx.SetHeader("Content-Type", "application/gzip")
				hctx.SetHeader("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, in.Name))
				if err := s.WriteTarGz(hctx.BodyWriter(), in.Name); err != nil {
					d.Logger.Warn("backup download failed", "backup", in.Name, "err", err)
				}
			}}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteBackup", Method: http.MethodDelete, Path: apiPrefix + "/backups/{name}",
		Summary: "Delete a backup", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *fileNamePath) (*noContent, error) {
			s, err := store(ctx, d, true)
			if err == nil {
				err = s.Remove(in.Name)
				d.Tasks.Record(ctx, ds.OpDeleteBackup, in.Name, err)
			}
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listLDIFFiles", Method: http.MethodGet, Path: apiPrefix + "/ldif-files",
		Summary: "Files in nsslapd-ldifdir (needs file access)", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*fileListOutput, error) {
			s, err := store(ctx, d, false)
			if err != nil {
				return nil, toHTTPError(err)
			}
			list, err := s.List(false)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &fileListOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "downloadLDIFFile", Method: http.MethodGet, Path: apiPrefix + "/ldif-files/{name}",
		Summary: "Download an LDIF file", Tags: tags,
		Responses: map[string]*huma.Response{"200": {Description: "LDIF", Content: map[string]*huma.MediaType{"text/ldif": {}}}}},
		func(ctx context.Context, in *fileNamePath) (*huma.StreamResponse, error) {
			s, err := store(ctx, d, false)
			if err != nil {
				return nil, toHTTPError(err)
			}
			f, info, err := s.OpenFile(in.Name)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &huma.StreamResponse{Body: func(hctx huma.Context) {
				defer func() { _ = f.Close() }()
				hctx.SetHeader("Content-Type", "text/ldif; charset=utf-8")
				hctx.SetHeader("Content-Length", fmt.Sprint(info.Size))
				hctx.SetHeader("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name))
				_, _ = io.Copy(hctx.BodyWriter(), f)
			}}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteLDIFFile", Method: http.MethodDelete, Path: apiPrefix + "/ldif-files/{name}",
		Summary: "Delete an LDIF file", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *fileNamePath) (*noContent, error) {
			s, err := store(ctx, d, false)
			if err == nil {
				err = s.Remove(in.Name)
				d.Tasks.Record(ctx, ds.OpDeleteLDIF, in.Name, err)
			}
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "exportBackend", Method: http.MethodPost, Path: apiPrefix + "/backends/{name}/export",
		Summary: "Export a backend to an LDIF file on the server (task)", Tags: tags, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *exportTaskInput) (*taskOutput, error) {
			file := in.Body.File
			if file == "" {
				file = in.Name + "_" + stamp() + ".ldif"
			}
			if !files.ValidName(file) {
				return nil, toHTTPError(fmt.Errorf("%w: %q", files.ErrInvalidName, file))
			}
			_, ldif, err := serverDirs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			attrs := map[string][]string{"nsInstance": {in.Name}, "nsFilename": {filepath.Join(ldif, file)}}
			if in.Body.Replication {
				attrs["nsExportReplica"] = []string{"true"}
			}
			return startTask(ctx, ds.TaskExport, in.Name+" → "+file, attrs)
		})

	huma.Register(api, huma.Operation{OperationID: "importBackend", Method: http.MethodPost, Path: apiPrefix + "/backends/{name}/import",
		Summary: "Import an LDIF file into a backend, replacing its content (task)", Tags: tags, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *importTaskInput) (*taskOutput, error) {
			if !files.ValidName(in.Body.File) {
				return nil, toHTTPError(fmt.Errorf("%w: %q", files.ErrInvalidName, in.Body.File))
			}
			_, ldif, err := serverDirs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			if d.Files != nil {
				if s, err := files.Open(d.Files.Root, ldif); err == nil {
					if _, err := s.Stat(in.Body.File); err != nil {
						return nil, toHTTPError(err)
					}
				}
			}
			return startTask(ctx, ds.TaskImport, in.Body.File+" → "+in.Name, map[string][]string{
				"nsInstance": {in.Name}, "nsFilename": {filepath.Join(ldif, in.Body.File)}})
		})
}

// uploadLDIF streams a request body into the LDIF directory:
// PUT /api/v1/ldif-files/{name}. Kept outside huma so large files are not
// buffered in memory.
func uploadLDIF(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := store(r.Context(), d, false)
		if err == nil {
			var info files.Info
			info, err = s.Create(chi.URLParam(r, "name"), r.Body)
			d.Tasks.Record(r.Context(), ds.OpUploadLDIF, chi.URLParam(r, "name"), err)
			if err == nil {
				writeJSON(w, http.StatusCreated, info)
				return
			}
		}
		var p *Problem
		if errorsAs(toHTTPError(err), &p) {
			writeProblem(w, p.Status, p.Detail)
			return
		}
		writeProblem(w, http.StatusInternalServerError, err.Error())
	}
}

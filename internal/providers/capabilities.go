package providers

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Capabilities tells the UI which feature groups are available in this deployment.
type Capabilities struct {
	Mode      string `json:"mode" enum:"sidecar,remote"`
	LDAPAuth  string `json:"ldapAuth" enum:"sasl-external,simple,anonymous" doc:"How ds-ui authenticates to the server"`
	Lifecycle string `json:"lifecycle" enum:"docker,none" doc:"Server process control (restart/stop/start)"`
	Files     bool   `json:"files" doc:"Access to logs, LDIF and backup files"`
	Certs     bool   `json:"certs" doc:"Management of the NSS certificate database"`
	Offline   bool   `json:"offline" doc:"Offline database operations (dsctl)"`
	Auth      string `json:"auth" enum:"none,ldap" doc:"How users sign in to ds-ui"`
}

// Files gives access to the server's data volume.
type Files struct {
	Root string
}

// DetectFiles returns a Files provider if dataDir looks like a 389 container
// volume readable by this process, or nil otherwise.
func DetectFiles(dataDir string) *Files {
	if dataDir == "" {
		return nil
	}
	for _, sub := range []string{"config", "logs"} {
		if !isReadableDir(filepath.Join(dataDir, sub)) {
			return nil
		}
	}
	return &Files{Root: dataDir}
}

// DetectCerts reports whether the NSS database and certutil are available.
func DetectCerts(files *Files) bool {
	if files == nil {
		return false
	}
	if _, err := exec.LookPath("certutil"); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(files.Root, "config", "cert9.db"))
	return err == nil
}

func isReadableDir(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	return err == nil && st.IsDir()
}

<script setup lang="ts">
/** Servers of the LDAP Pass-Through Authentication plug-in (its plug-in arguments). */
import { computed, h, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NDataTable,
  NDynamicTags,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSpace,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { Pencil, Plus, Trash2 } from "@lucide/vue";
import { api, call, type Schemas } from "@/api/client";
import { localizeError, type LocalizedError } from "@/i18n/errors";
import { useAuthStore } from "@/stores/auth";
import { useServerStore } from "@/stores/server";

type Server = Schemas["PassThruServer"];

const { t } = useI18n();
const message = useMessage();
const auth = useAuthStore();
const server = useServerStore();

const list = ref<Server[]>();
const error = ref<LocalizedError>();
async function load() {
  try {
    list.value = await call(api.GET("/api/v1/plugins/passthru/servers"));
  } catch (e) {
    error.value = localizeError(e);
  }
}
onMounted(load);

async function save(servers: Server[]) {
  list.value = await call(
    api.PUT("/api/v1/plugins/passthru/servers", { body: servers }),
  );
  void server.refreshState();
  message.success(t("passthru.saved"), { duration: 6000 });
}

const blank = (): Server => ({
  secure: false,
  hosts: [],
  subtree: "",
  maxConns: 3,
  maxOps: 5,
  timeout: 300,
  ldapVersion: 3,
  connLifetime: 300,
  startTLS: false,
});
const editing = ref<{ index: number; s: Server }>();
const busy = ref(false);
const formError = ref<LocalizedError>();
function edit(index: number) {
  formError.value = undefined;
  editing.value = {
    index,
    s:
      index < 0
        ? blank()
        : { ...list.value![index]!, hosts: [...list.value![index]!.hosts] },
  };
}
async function submit() {
  const e = editing.value;
  if (!e) return;
  const next = [...(list.value ?? [])];
  if (e.index < 0) next.push(e.s);
  else next[e.index] = e.s;
  busy.value = true;
  try {
    await save(next);
    editing.value = undefined;
  } catch (err) {
    formError.value = localizeError(err);
  } finally {
    busy.value = false;
  }
}
async function remove(index: number) {
  try {
    await save(list.value!.filter((_, i) => i !== index));
  } catch (err) {
    message.error(localizeError(err).summary);
  }
}

const url = (s: Server) =>
  `${s.secure ? "ldaps" : "ldap"}://${s.hosts.join(" ")}/`;
const columns = computed<DataTableColumns<Server>>(() => [
  {
    key: "subtree",
    title: t("passthru.subtree"),
    render: (s) =>
      h("div", null, [
        h("div", { class: "mono" }, s.subtree),
        h("div", { class: "mono", style: "font-size:12px;opacity:.7" }, url(s)),
      ]),
  },
  {
    key: "actions",
    title: "",
    width: 80,
    render: (_s, i) =>
      auth.readOnly
        ? null
        : h(NSpace, { size: 4, wrap: false }, () => [
            h(
              NButton,
              {
                size: "tiny",
                quaternary: true,
                "aria-label": t("passthru.edit"),
                onClick: () => edit(i),
              },
              { icon: () => h(NIcon, null, () => h(Pencil)) },
            ),
            h(
              NButton,
              {
                size: "tiny",
                quaternary: true,
                "aria-label": t("passthru.remove"),
                onClick: () => remove(i),
              },
              { icon: () => h(NIcon, null, () => h(Trash2)) },
            ),
          ]),
  },
]);
</script>

<template>
  <div>
    <NCard size="small" :title="t('passthru.title')">
      <template v-if="!auth.readOnly" #header-extra>
        <NButton size="small" @click="edit(-1)">
          <template #icon
            ><NIcon><Plus /></NIcon
          ></template>
          {{ t("passthru.add") }}
        </NButton>
      </template>
      <p class="hint">{{ t("passthru.description") }}</p>
      <NAlert v-if="error" type="error"
        >{{ error.summary }}
        <div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert
      >
      <NDataTable
        v-else
        :columns="columns"
        :data="list ?? []"
        :loading="!list"
        size="small"
      >
        <template #empty
          ><NEmpty size="small" :description="t('passthru.none')"
        /></template>
      </NDataTable>
    </NCard>

    <NModal
      :show="!!editing"
      preset="card"
      :title="
        editing && editing.index >= 0 ? t('passthru.edit') : t('passthru.add')
      "
      :style="{ width: 'min(620px, 94vw)' }"
      :mask-closable="!busy"
      @update:show="(v: boolean) => !v && (editing = undefined)"
    >
      <NForm v-if="editing" label-placement="top" @submit.prevent="submit">
        <NFormItem
          :label="t('passthru.subtree')"
          :feedback="t('passthru.subtreeHint')"
        >
          <NInput
            v-model:value="editing.s.subtree"
            :input-props="{ class: 'mono', spellcheck: false }"
            placeholder="ou=People,dc=corp,dc=example"
          />
        </NFormItem>
        <NFormItem
          :label="t('passthru.hosts')"
          :feedback="t('passthru.hostsHint')"
        >
          <NDynamicTags v-model:value="editing.s.hosts" />
        </NFormItem>
        <NFormItem :label="t('passthru.protocol')">
          <NSpace align="center">
            <NRadioGroup v-model:value="editing.s.secure">
              <NRadioButton :value="false">ldap://</NRadioButton>
              <NRadioButton :value="true">ldaps://</NRadioButton>
            </NRadioGroup>
            <NCheckbox
              v-if="!editing.s.secure"
              v-model:checked="editing.s.startTLS"
              >StartTLS</NCheckbox
            >
          </NSpace>
        </NFormItem>
        <div class="grid">
          <NFormItem :label="t('passthru.maxConns')"
            ><NInputNumber v-model:value="editing.s.maxConns" :min="1"
          /></NFormItem>
          <NFormItem :label="t('passthru.maxOps')"
            ><NInputNumber v-model:value="editing.s.maxOps" :min="1"
          /></NFormItem>
          <NFormItem :label="t('passthru.timeout')"
            ><NInputNumber v-model:value="editing.s.timeout" :min="0"
          /></NFormItem>
          <NFormItem :label="t('passthru.connLifetime')"
            ><NInputNumber v-model:value="editing.s.connLifetime" :min="0"
          /></NFormItem>
        </div>
      </NForm>
      <NAlert v-if="formError" type="error"
        >{{ formError.summary }}
        <div v-if="formError.detail" class="mono">
          {{ formError.detail }}
        </div></NAlert
      >
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="busy" @click="editing = undefined">{{
            t("edit.cancel")
          }}</NButton>
          <NButton
            type="primary"
            :loading="busy"
            :disabled="!editing?.s.subtree.trim() || !editing?.s.hosts.length"
            @click="submit"
          >
            {{ t("pages._save") }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  opacity: 0.75;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 0 12px;
}
</style>

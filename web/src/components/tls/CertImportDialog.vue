<script setup lang="ts">
/**
 * Adding certificates: a CA, a certificate with its key (PKCS#12 or PEM), a
 * certificate request, or the certificate issued for that request.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDynamicTags, NForm, NFormItem, NInput, NModal, NRadioButton, NRadioGroup, NSelect, NSpace, useMessage } from 'naive-ui'
import { api, call } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

export type ImportKind = 'ca' | 'pair' | 'csr' | 'issued'

const props = defineProps<{ kind: ImportKind }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const message = useMessage()

const nickname = ref('')
const pem = ref('')
const pairFormat = ref<'pkcs12' | 'pem'>('pkcs12')
const p12 = ref<string>() // base64
const p12Name = ref('')
const password = ref('')
const key = ref('')
const chain = ref('')
const subject = ref('')
const dnsNames = ref<string[]>([])
const keyType = ref<'rsa2048' | 'rsa3072' | 'rsa4096' | 'ec256' | 'ec384'>('rsa3072')
const csr = ref('')
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (!v) return
  nickname.value = pem.value = key.value = chain.value = password.value = subject.value = csr.value = p12Name.value = ''
  p12.value = undefined
  dnsNames.value = []
  error.value = undefined
})

function readFile(e: Event, as: 'text' | 'base64', set: (v: string, name: string) => void) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  const r = new FileReader()
  r.onload = () => {
    const res = r.result as string
    set(as === 'base64' ? res.slice(res.indexOf(',') + 1) : res, f.name)
  }
  if (as === 'base64') r.readAsDataURL(f)
  else r.readAsText(f)
}

const valid = computed(() => {
  switch (props.kind) {
    case 'ca':
    case 'issued':
      return !!nickname.value.trim() && pem.value.includes('BEGIN CERTIFICATE')
    case 'pair':
      return pairFormat.value === 'pkcs12' ? !!p12.value : !!nickname.value.trim() && pem.value.includes('BEGIN CERTIFICATE') && key.value.includes('PRIVATE KEY')
    case 'csr':
      return subject.value.includes('=')
  }
  return false
})

const title = computed(() => t(`tls.import.${props.kind}`))

async function submit() {
  busy.value = true
  error.value = undefined
  try {
    switch (props.kind) {
      case 'ca':
        await call(api.POST('/api/v1/security/certs/ca', { body: { nickname: nickname.value.trim(), pem: pem.value } }))
        break
      case 'issued':
        await call(api.POST('/api/v1/security/certs/issued', { body: { nickname: nickname.value.trim(), pem: pem.value } }))
        break
      case 'pair': {
        const res =
          pairFormat.value === 'pkcs12'
            ? await call(api.POST('/api/v1/security/certs/pkcs12', { body: { data: p12.value!, password: password.value } }))
            : await call(api.POST('/api/v1/security/certs/pem-pair', { body: { nickname: nickname.value.trim(), cert: pem.value, key: key.value, chain: chain.value } }))
        message.info(t('tls.imported', { names: res.nicknames.join(', ') || '—' }))
        break
      }
      case 'csr': {
        const res = await call(api.POST('/api/v1/security/csr', { body: { subject: subject.value.trim(), dnsNames: dnsNames.value, keyType: keyType.value } }))
        csr.value = res.pem
        emit('done')
        return // keep the dialog open to copy the request
      }
    }
    message.success(t('tls.saved'))
    show.value = false
    emit('done')
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

async function copyCSR() {
  await navigator.clipboard.writeText(csr.value)
  message.success(t('tls.copied'))
}
function downloadCSR() {
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([csr.value], { type: 'application/pkcs10' }))
  a.download = 'server.csr'
  a.click()
  URL.revokeObjectURL(a.href)
}
const keyTypes = ['rsa2048', 'rsa3072', 'rsa4096', 'ec256', 'ec384'].map((v) => ({ label: t(`tls.keyTypes.${v}`), value: v }))
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="title" :style="{ width: 'min(720px, 94vw)' }" :mask-closable="!busy">
    <p class="hint">{{ t(`tls.importHints.${kind}`) }}</p>
    <NForm label-placement="top" @submit.prevent="submit">
      <template v-if="kind === 'csr'">
        <template v-if="!csr">
          <NFormItem :label="t('tls.subject')" :feedback="t('tls.subjectHint')">
            <NInput v-model:value="subject" placeholder="CN=ldap.example.org,O=Example" :input-props="{ class: 'mono', spellcheck: false }" />
          </NFormItem>
          <NFormItem :label="t('tls.dnsNames')" :feedback="t('tls.dnsNamesHint')"><NDynamicTags v-model:value="dnsNames" /></NFormItem>
          <NFormItem :label="t('tls.keyType')"><NSelect v-model:value="keyType" :options="keyTypes" class="w" /></NFormItem>
        </template>
        <template v-else>
          <NAlert type="success" class="block">{{ t('tls.csrReady') }}</NAlert>
          <NInput :value="csr" type="textarea" readonly :autosize="{ minRows: 8, maxRows: 16 }" :input-props="{ class: 'mono' }" />
          <NSpace class="block">
            <NButton @click="copyCSR">{{ t('tls.copy') }}</NButton>
            <NButton @click="downloadCSR">{{ t('tls.download') }}</NButton>
          </NSpace>
        </template>
      </template>

      <template v-else>
        <NFormItem v-if="kind === 'pair'" :label="t('tls.format')">
          <NRadioGroup v-model:value="pairFormat">
            <NRadioButton value="pkcs12">PKCS#12 (.p12, .pfx)</NRadioButton>
            <NRadioButton value="pem">PEM</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <template v-if="kind === 'pair' && pairFormat === 'pkcs12'">
          <NFormItem :label="t('tls.file')">
            <input type="file" accept=".p12,.pfx" @change="(e) => readFile(e, 'base64', (v, n) => ((p12 = v), (p12Name = n)))" />
          </NFormItem>
          <NFormItem :label="t('tls.p12Password')">
            <NInput v-model:value="password" type="password" show-password-on="click" :input-props="{ autocomplete: 'off' }" />
          </NFormItem>
        </template>
        <template v-else>
          <NFormItem :label="t('tls.nickname')" :feedback="t('tls.nicknameHint')"><NInput v-model:value="nickname" /></NFormItem>
          <NFormItem :label="t('tls.certificatePEM')">
            <div class="pem">
              <input type="file" accept=".pem,.crt,.cer" @change="(e) => readFile(e, 'text', (v) => (pem = v))" />
              <NInput v-model:value="pem" type="textarea" :autosize="{ minRows: 4, maxRows: 10 }" placeholder="-----BEGIN CERTIFICATE-----" :input-props="{ class: 'mono', spellcheck: false }" />
            </div>
          </NFormItem>
          <template v-if="kind === 'pair'">
            <NFormItem :label="t('tls.keyPEM')">
              <div class="pem">
                <input type="file" accept=".pem,.key" @change="(e) => readFile(e, 'text', (v) => (key = v))" />
                <NInput v-model:value="key" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" placeholder="-----BEGIN PRIVATE KEY-----" :input-props="{ class: 'mono', spellcheck: false }" />
              </div>
            </NFormItem>
            <NFormItem :label="t('tls.chainPEM')">
              <NInput v-model:value="chain" type="textarea" :autosize="{ minRows: 2, maxRows: 8 }" :input-props="{ class: 'mono', spellcheck: false }" />
            </NFormItem>
          </template>
        </template>
      </template>
    </NForm>
    <NAlert v-if="error" type="error">{{ error.summary }}<div v-if="error.detail" class="mono detail">{{ error.detail }}</div></NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ csr ? t('tls.close') : t('edit.cancel') }}</NButton>
        <NButton v-if="!csr" type="primary" :disabled="!valid" :loading="busy" @click="submit">
          {{ kind === 'csr' ? t('tls.generate') : t('tls.importButton') }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 13px;
}

.pem {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.block {
  margin: 10px 0;
}

.w {
  width: 260px;
}

.detail {
  white-space: pre-wrap;
}
</style>

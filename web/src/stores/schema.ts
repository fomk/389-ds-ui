import { defineStore } from 'pinia'
import { shallowRef } from 'vue'
import { api, call, type AttributeType, type LdapSchema, type ObjectClass } from '@/api/client'

/**
 * LDAP schema with lookup by name/OID. Loaded once; the browser revalidates it
 * with ETag (the API answers 304 when unchanged).
 */
export const useSchemaStore = defineStore('schema', () => {
  const schema = shallowRef<LdapSchema>()
  let attrs = new Map<string, AttributeType>()
  let classes = new Map<string, ObjectClass>()
  let loading: Promise<void> | undefined

  function index(s: LdapSchema) {
    attrs = new Map()
    for (const a of s.attributeTypes) {
      attrs.set(a.oid.toLowerCase(), a)
      for (const n of a.names) attrs.set(n.toLowerCase(), a)
    }
    classes = new Map()
    for (const c of s.objectClasses) {
      classes.set(c.oid.toLowerCase(), c)
      for (const n of c.names) classes.set(n.toLowerCase(), c)
    }
  }

  function load(force = false): Promise<void> {
    if (loading && !force) return loading
    loading = call(api.GET('/api/v1/schema')).then((s) => {
      index(s)
      schema.value = s
    })
    loading.catch(() => (loading = undefined))
    return loading
  }

  /** Attribute type by name (options like ";lang-ru" are ignored) or OID. */
  function attr(name: string): AttributeType | undefined {
    void schema.value // reactive dependency
    return attrs.get(name.split(';')[0]!.toLowerCase())
  }

  function objectClass(name: string): ObjectClass | undefined {
    void schema.value
    return classes.get(name.toLowerCase())
  }

  return { schema, load, attr, objectClass }
})

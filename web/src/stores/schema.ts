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

  /** The class and all its superclasses, nearest first (cycle-safe). */
  function superclasses(name: string): ObjectClass[] {
    const out: ObjectClass[] = []
    const seen = new Set<ObjectClass>()
    const walk = (n: string) => {
      const c = objectClass(n)
      if (!c || seen.has(c)) return
      seen.add(c)
      out.push(c)
      for (const s of c.sup ?? []) walk(s)
    }
    walk(name)
    return out
  }

  /**
   * MUST and MAY attributes (canonical names) allowed by a set of object
   * classes, including inherited ones; `any` is true for extensibleObject.
   */
  function allowed(objectClasses: string[]): { must: string[]; may: string[]; any: boolean } {
    const canon = (n: string) => attr(n)?.names[0] ?? n
    const must = new Map<string, string>()
    const may = new Map<string, string>()
    let any = false
    const chains = objectClasses.map(superclasses)
    for (const chain of chains) {
      for (const c of chain) {
        if (c.names.some((n) => n.toLowerCase() === 'extensibleobject')) any = true
        for (const n of c.must) must.set(canon(n).toLowerCase(), canon(n))
      }
    }
    for (const chain of chains) {
      for (const c of chain) {
        for (const n of c.may) {
          const key = canon(n).toLowerCase()
          if (!must.has(key)) may.set(key, canon(n))
        }
      }
    }
    const sorted = (m: Map<string, string>) => [...m.values()].sort((a, b) => a.localeCompare(b))
    return { must: sorted(must), may: sorted(may), any }
  }

  return { schema, load, attr, objectClass, superclasses, allowed }
})

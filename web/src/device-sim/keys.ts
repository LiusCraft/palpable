// 设备密钥：每只玩偶一把 ECDSA P-256 私钥，不可导出，存在本浏览器的 IndexedDB。
// 真实设备的私钥在出厂时写入并受 Flash 加密保护；握手规则见 docs/trust.md §2。
import type { MAC } from '../shared/protocol.gen'

const DB = 'palpable.keys'
const STORE = 'keys'
const ALG = { name: 'ECDSA', namedCurve: 'P-256' } as const

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error ?? new Error('indexedDB open failed'))
  })
}

function tx<T>(db: IDBDatabase, mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    const req = fn(db.transaction(STORE, mode).objectStore(STORE))
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error ?? new Error('indexedDB request failed'))
  })
}

const cache = new Map<MAC, Promise<CryptoKeyPair>>()

/** 取这只玩偶的密钥对；没有就生成一把。浏览器存储不可用时退化为本页内存，刷新后等于换了一台设备。 */
export function deviceKey(mac: MAC): Promise<CryptoKeyPair> {
  let p = cache.get(mac)
  if (!p) {
    p = loadOrCreate(mac)
    cache.set(mac, p)
  }
  return p
}

async function loadOrCreate(mac: MAC): Promise<CryptoKeyPair> {
  let db: IDBDatabase | null = null
  try {
    db = await openDB()
    const saved = await tx<CryptoKeyPair | undefined>(db, 'readonly', (s) => s.get(mac) as IDBRequest<CryptoKeyPair | undefined>)
    if (saved) return saved
  } catch {
    db = null
  }
  const pair = await crypto.subtle.generateKey(ALG, false, ['sign', 'verify'])
  if (db) {
    try {
      await tx(db, 'readwrite', (s) => s.put(pair, mac))
    } catch {
      // 存不下也能用，只是下次打开会被当成另一台设备
    }
  }
  return pair
}

function b64(bytes: ArrayBuffer): string {
  return btoa(String.fromCharCode(...new Uint8Array(bytes)))
}

function b64url(bytes: ArrayBuffer): string {
  return b64(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/** hello.pubkey：base64(SPKI DER)。 */
export async function publicKeyB64(mac: MAC): Promise<string> {
  const { publicKey } = await deviceKey(mac)
  return b64(await crypto.subtle.exportKey('spki', publicKey))
}

/** hello.proof.signature：WebCrypto 的 ECDSA 输出正是协议要求的 r‖s 64 字节。 */
export async function signChallenge(mac: MAC, nonce: string): Promise<string> {
  const { privateKey } = await deviceKey(mac)
  const msg = new TextEncoder().encode(`palpable-auth-v1\n${mac}\n${nonce}`)
  return b64url(await crypto.subtle.sign({ name: 'ECDSA', hash: 'SHA-256' }, privateKey, msg))
}

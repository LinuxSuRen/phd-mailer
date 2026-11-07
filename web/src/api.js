const TIMEOUT = 15000

export class ApiError extends Error {
  constructor(message, code) {
    super(message)
    this.code = code
  }
}

async function request(path, options = {}) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), TIMEOUT)
  let resp
  try {
    resp = await fetch(path, { ...options, signal: controller.signal })
  } catch (err) {
    if (err.name === 'AbortError') {
      throw new ApiError('请求超时，请检查网络后重试', 'timeout')
    }
    throw new ApiError('无法连接后端服务，请确认服务已启动', 'network')
  } finally {
    clearTimeout(timer)
  }
  let body = null
  try {
    body = await resp.json()
  } catch {
    // 空响应或非 JSON，容错处理
  }
  if (!resp.ok) {
    const msg = body?.error?.message || `请求失败（HTTP ${resp.status}）`
    throw new ApiError(msg, `HTTP${resp.status}`)
  }
  return body?.data ?? body
}

export const getContacts = () => request('/api/contacts')
export const getConfig = () => request('/api/config')
export const updateConfig = (cfg) =>
  request('/api/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  })
export const sendEmails = (ids, dryRun) =>
  request('/api/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids, dry_run: dryRun }),
  })
export const getPrompt = () => request('/api/prompt')
export const importContacts = (content) =>
  request('/api/contacts/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
export const exportData = () => request('/api/data/export')
export const savePrompt = (content) =>
  request('/api/prompt', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })

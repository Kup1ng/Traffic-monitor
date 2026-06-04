// Shared auth state plus session refresh / login / logout helpers.
export function useAuth() {
  const authenticated = useState<boolean>('authenticated', () => false)
  const passwordConfigured = useState<boolean>('passwordConfigured', () => true)
  const checked = useState<boolean>('authChecked', () => false)
  const api = useApi()

  async function refresh(): Promise<boolean> {
    try {
      const s = await api.getSession()
      authenticated.value = s.authenticated
      passwordConfigured.value = s.password_configured
    } catch {
      authenticated.value = false
    }
    checked.value = true
    return authenticated.value
  }

  async function login(password: string): Promise<void> {
    await api.login(password)
    authenticated.value = true
  }

  async function logout(): Promise<void> {
    try {
      await api.logout()
    } finally {
      authenticated.value = false
    }
  }

  return { authenticated, passwordConfigured, checked, refresh, login, logout }
}

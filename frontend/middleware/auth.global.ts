// Client-side route guard (SPA): unauthenticated users go to /login; an
// authenticated user landing on /login is sent to the dashboard.
export default defineNuxtRouteMiddleware(async (to) => {
  const { authenticated, checked, refresh } = useAuth()

  if (!checked.value) {
    await refresh()
  }

  if (!authenticated.value && to.path !== '/login') {
    return navigateTo('/login')
  }
  if (authenticated.value && to.path === '/login') {
    return navigateTo('/')
  }
})

import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

// Retheme PrimeVue's primary color to clay violet so buttons and active controls
// match the claymorphism palette.
const TrafficPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{violet.50}',
      100: '{violet.100}',
      200: '{violet.200}',
      300: '{violet.300}',
      400: '{violet.400}',
      500: '{violet.500}',
      600: '{violet.600}',
      700: '{violet.700}',
      800: '{violet.800}',
      900: '{violet.900}',
      950: '{violet.950}',
    },
  },
})

// Static SPA. `nuxt generate` produces files under .output/public, which are
// copied into the Go binary's embed directory (backend/web/public).
//
// The base URL is baked at build time from TM_BUILD_BASE. Production builds set
// it to a placeholder ("/__TM_BASE__/") that the Go server rewrites at startup
// to the secret base path chosen at install time, so one binary can be served
// under any path without rebuilding. Dev/default is root ("/").
const BUILD_BASE = process.env.TM_BUILD_BASE || '/'

export default defineNuxtConfig({
  ssr: false,
  compatibilityDate: '2025-01-01',
  devtools: { enabled: false },

  app: {
    baseURL: BUILD_BASE,
    head: {
      title: 'Traffic Monitor',
      htmlAttrs: { lang: 'en' },
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'color-scheme', content: 'light' },
        { name: 'description', content: 'Lightweight self-hosted network traffic monitor' },
      ],
      link: [{ rel: 'icon', type: 'image/svg+xml', href: 'favicon.svg' }],
    },
  },

  modules: ['@primevue/nuxt-module', '@nuxtjs/tailwindcss'],

  // Tailwind v3: our main.css declares the layer order and the @tailwind
  // directives so PrimeVue's styles sit between Tailwind base and utilities.
  tailwindcss: {
    cssPath: '~/assets/css/main.css',
    viewer: false,
  },

  css: [
    '@fontsource/fira-sans/400.css',
    '@fontsource/fira-sans/500.css',
    '@fontsource/fira-sans/600.css',
    '@fontsource/fira-sans/700.css',
    '@fontsource/fira-mono/400.css',
    '@fontsource/fira-mono/500.css',
    '@fontsource/fira-mono/700.css',
    'primeicons/primeicons.css',
  ],

  primevue: {
    options: {
      ripple: true,
      theme: {
        preset: TrafficPreset,
        options: {
          darkModeSelector: '.dark',
          cssLayer: { name: 'primevue', order: 'tailwind-base, primevue, tailwind-utilities' },
        },
      },
    },
    components: { exclude: ['Editor'] }, // auto-import <Chart>, skip <Editor> (needs quill)
  },

  // During `nuxt dev`, forward /api to the Go backend so there is no CORS and
  // the SSE stream works the same as in production.
  nitro: {
    devProxy: {
      '/api': { target: 'http://127.0.0.1:8088', changeOrigin: true },
    },
  },
})

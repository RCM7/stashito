import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Stashito',
  description: 'Pull-through cache for Docker/OCI images. Pull once — every later pull is served from your own disk.',
  base: '/docs/',
  appearance: 'force-dark',
  cleanUrls: true,
  head: [
    ['link', { rel: 'icon', href: '/docs/favicon.ico', sizes: '48x48' }],
    ['link', { rel: 'icon', href: '/docs/favicon.svg', type: 'image/svg+xml' }],
    ['link', { rel: 'apple-touch-icon', href: '/docs/apple-touch-icon.png' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;700&display=swap' }]
  ],
  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'stashito.com', link: 'https://stashito.com' },
      { text: 'Docker Hub', link: 'https://hub.docker.com/r/rcm7/stashito' }
    ],
    sidebar: [
      {
        items: [
          { text: 'what is stashito', link: '/' },
          { text: 'quickstart', link: '/quickstart' },
          { text: 'configuration', link: '/configuration' },
          { text: 'registries', link: '/registries' },
          { text: 'kubernetes', link: '/kubernetes' },
          { text: 'api', link: '/api' },
          { text: 'observability', link: '/observability' }
        ]
      }
    ],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    docFooter: { prev: false, next: false }
  }
})

// 运行时配置（本地开发默认值，Docker 环境会被 entrypoint 脚本覆盖）
window.__RUNTIME_CONFIG__ = {
  MAX_FILE_SIZE_MB: 50,
  MAX_SKILL_BUNDLE_SIZE_MB: 256,
  // Optional: serve embed on a dedicated origin, e.g. 'https://embed.example.com'
  EMBED_BASE_URL: '',
  // Optional: default UI locale for first-time visitors (zh-CN | en-US | ru-RU | ko-KR | ja-JP)
  DEFAULT_LOCALE: '',
  // Optional: product display name (letters, digits, space, . _ -, max 32 chars). Empty = "WeKnora".
  APP_NAME: '',
  // Optional: cloud service display name. Empty = "WeKnora Cloud".
  CLOUD_SERVICE_NAME: '',
  // Optional: overrides the <meta name="description"> content wholesale. Empty keeps the built-in text.
  APP_DESCRIPTION: '',
  // Optional: brand logo URL (sidebar + login page). Accepts http(s) URL, same-origin root
  // path (e.g. '/branding/logo.png' via volume mount) or data:image URI. Empty = bundled WeKnora logo.
  APP_LOGO_URL: '',
  // Optional: favicon URL (same acceptance rules). Empty keeps the bundled favicon.ico.
  APP_FAVICON_URL: '',
};

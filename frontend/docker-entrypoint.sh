#!/bin/sh

# Only emit whitelisted locale tags to avoid config.js injection from env values.
RUNTIME_DEFAULT_LOCALE=""
case "${DEFAULT_LOCALE:-}" in
  zh-CN|en-US|ru-RU|ko-KR|ja-JP) RUNTIME_DEFAULT_LOCALE="${DEFAULT_LOCALE}" ;;
esac

# 品牌参数：产品名/云服务名只允许安全字符集（防 config.js 注入），留空则前端回退默认值。
sanitize_brand() {
  printf '%s' "${1:-}" | tr -d '\r\n' | tr -cd 'A-Za-z0-9 ._-' | cut -c 1-32
}
RUNTIME_APP_NAME=$(sanitize_brand "${APP_NAME:-}")
RUNTIME_CLOUD_SERVICE_NAME=$(sanitize_brand "${CLOUD_SERVICE_NAME:-}")

# 描述是自由文本（可含中文等），单独做 JS 字符串转义：
# 先转义反斜杠/引号，再把 $ 转成 \$ 以躲开下方非引用 heredoc 的变量展开。
RUNTIME_APP_DESCRIPTION=$(printf '%s' "${APP_DESCRIPTION:-}" \
  | tr -d '\r\n' \
  | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/\$/\\$/g')

# Logo / favicon 是 URL：先剔除会破坏 JS 字符串的字符，再用前缀白名单校验
# （只允许 http(s)、同源根路径、data:image），挡掉 javascript: 等危险协议。
sanitize_asset_url() {
  _v=$(printf '%s' "${1:-}" | tr -d '\r\n"\\`$'"'" | cut -c 1-8192)
  case "$_v" in
    http://*|https://*|/*|data:image/*) printf '%s' "$_v" ;;
    *) printf '' ;;
  esac
}
RUNTIME_APP_LOGO_URL=$(sanitize_asset_url "${APP_LOGO_URL:-}")
RUNTIME_APP_FAVICON_URL=$(sanitize_asset_url "${APP_FAVICON_URL:-}")

# 生成运行时配置文件，注入环境变量到前端
FILE_MB=${MAX_FILE_SIZE_MB:-50}
SKILL_MB=${MAX_SKILL_BUNDLE_SIZE_MB:-256}
if [ "$SKILL_MB" -lt "$FILE_MB" ] 2>/dev/null; then
  SKILL_MB=$FILE_MB
fi
if [ "$SKILL_MB" -gt 512 ] 2>/dev/null; then
  SKILL_MB=512
fi

cat > /usr/share/nginx/html/config.js << EOF
window.__RUNTIME_CONFIG__ = {
  MAX_FILE_SIZE_MB: ${FILE_MB},
  MAX_SKILL_BUNDLE_SIZE_MB: ${SKILL_MB},
  DEFAULT_LOCALE: "${RUNTIME_DEFAULT_LOCALE}",
  APP_NAME: "${RUNTIME_APP_NAME}",
  CLOUD_SERVICE_NAME: "${RUNTIME_CLOUD_SERVICE_NAME}",
  APP_DESCRIPTION: "${RUNTIME_APP_DESCRIPTION}",
  APP_LOGO_URL: "${RUNTIME_APP_LOGO_URL}",
  APP_FAVICON_URL: "${RUNTIME_APP_FAVICON_URL}"
};
EOF

# 处理 nginx 配置。
# 两个上限分开注入：全站保持知识库的 MAX_FILE_SIZE，只有技能 zip 上传的两条
# 集合路由放宽到 MAX_SKILL_BUNDLE_SIZE（不含 /install、PATCH 等子路径）。
# 合成一个全站上限会让每个上传端点都能收到技能包那么大的 body。
export MAX_FILE_SIZE=${FILE_MB}M
export MAX_SKILL_BUNDLE_SIZE=${SKILL_MB}M
export APP_HOST=${APP_HOST:-app}
export APP_PORT=${APP_PORT:-8080}
export APP_SCHEME=${APP_SCHEME:-http}
envsubst '${MAX_FILE_SIZE} ${MAX_SKILL_BUNDLE_SIZE} ${APP_HOST} ${APP_PORT} ${APP_SCHEME}' \
  < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf

# 启动 nginx
exec nginx -g 'daemon off;'

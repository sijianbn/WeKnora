(() => {
  const scriptClass = 't-svg-js-stylesheet--unique-class';
  const linkClass = 't-iconfont-stylesheet--unique-class';
  const versions = ['0.4.0', '0.4.1', '0.4.2', '0.4.3', '0.4.4'];

  function install() {
    if (!document.body) {
      document.addEventListener('DOMContentLoaded', install, { once: true });
      return;
    }

    versions.forEach((version) => {
      const baseUrl = `https://tdesign.gtimg.com/icon/${version}/fonts/index`;
      const scriptUrl = `${baseUrl}.js`;
      if (!document.querySelector(`script.${scriptClass}[src="${scriptUrl}"]`)) {
        const script = document.createElement('script');
        script.className = scriptClass;
        script.src = scriptUrl;
        script.type = 'text/no-load';
        script.dataset.weknoraBlockedCdn = 'tdesign-icons';
        document.body.appendChild(script);
      }

      const linkUrl = `${baseUrl}.css`;
      if (!document.querySelector(`link.${linkClass}[href="${linkUrl}"]`)) {
        const link = document.createElement('link');
        link.className = linkClass;
        link.href = linkUrl;
        link.rel = 'preload-blocked';
        link.dataset.weknoraBlockedCdn = 'tdesign-icons';
        document.head.appendChild(link);
      }
    });
  }

  install();
})();
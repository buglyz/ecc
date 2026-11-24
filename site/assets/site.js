(function () {
  'use strict';

  var root = document.documentElement;
  var themeButton = document.getElementById('themeToggle');
  var themeToggleText = document.getElementById('themeToggleText');
  var themeIcon = document.getElementById('themeIcon');
  var shotButtons = Array.from(document.querySelectorAll('[data-shot]'));
  var heroShot = document.getElementById('heroShot');

  var shots = {
    light: {
      src: 'assets/dashboard-light.png',
      alt: 'ECC Fan Controller 本地 Web 控制台浅色界面：顶部实时温度与风扇上限遥测，下方左侧 5 点曲线编辑器与右侧时序监控图'
    },
    dark: {
      src: 'assets/dashboard-dark.png',
      alt: 'ECC Fan Controller 本地 Web 控制台深色界面：石墨灰中性面板，左侧 5 点曲线编辑器与右侧实时时序监控图'
    }
  };

  function selectShot(key) {
    var shot = shots[key];
    if (!shot || !heroShot) return;
    heroShot.src = shot.src;
    heroShot.alt = shot.alt;
    shotButtons.forEach(function (button) {
      var active = button.getAttribute('data-shot') === key;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
  }

  function updateThemeIcon(isDark) {
    if (!themeIcon) return;
    themeIcon.innerHTML = isDark
      ? '<circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"></path>'
      : '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"></path>';
  }

  function setTheme(theme) {
    var isDark = theme === 'dark';
    root.setAttribute('data-theme', isDark ? 'dark' : 'light');
    try {
      localStorage.setItem('ecc-site-theme', isDark ? 'dark' : 'light');
    } catch (e) {}

    if (themeToggleText) {
      themeToggleText.textContent = isDark ? '浅色' : '深色';
    }
    updateThemeIcon(isDark);

    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) {
      meta.setAttribute('content', isDark ? '#141415' : '#fafafa');
    }
    selectShot(isDark ? 'dark' : 'light');
  }

  shotButtons.forEach(function (button) {
    button.addEventListener('click', function () {
      selectShot(button.getAttribute('data-shot'));
    });
  });

  if (themeButton) {
    themeButton.addEventListener('click', function () {
      var current = root.getAttribute('data-theme') || 'light';
      setTheme(current === 'dark' ? 'light' : 'dark');
    });
  }

  var initialTheme = root.getAttribute('data-theme') || 'light';
  setTheme(initialTheme);

  var yearEl = document.getElementById('year');
  if (yearEl) {
    yearEl.textContent = String(new Date().getFullYear());
  }
})();

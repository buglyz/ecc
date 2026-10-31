(function () {
  'use strict';

  var root = document.documentElement;
  var themeButton = document.getElementById('themeToggle');
  var themeToggleText = document.getElementById('themeToggleText');
  var shotButtons = Array.from(document.querySelectorAll('[data-shot]'));
  var heroShot = document.getElementById('heroShot');

  var shots = {
    light: {
      src: 'assets/dashboard-light.png',
      alt: 'ECC Fan Controller 全新双擎工作台：左侧风扇阶梯曲线与5节点调音矩阵，右侧全景历史时序监控'
    },
    dark: {
      src: 'assets/dashboard-dark.png',
      alt: 'ECC Fan Controller 全新双擎工作台深色界面：工业暗黑调色板，左右并排对称布局'
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

  function setTheme(theme) {
    root.setAttribute('data-theme', theme);
    try {
      localStorage.setItem('ecc-site-theme', theme);
    } catch (e) {}

    if (themeToggleText) {
      themeToggleText.textContent = theme === 'dark' ? '浅色' : '深色';
    }
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) {
      meta.setAttribute('content', theme === 'dark' ? '#090d16' : '#ffffff');
    }
    selectShot(theme);
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

  // 初始化当前主题
  var initialTheme = root.getAttribute('data-theme') || 'light';
  setTheme(initialTheme);

  // 动态年份
  var yearEl = document.getElementById('year');
  if (yearEl) {
    yearEl.textContent = String(new Date().getFullYear());
  }
})();

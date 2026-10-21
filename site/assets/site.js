(function () {
  const root = document.documentElement;
  const themeButton = document.getElementById("themeToggle");
  const shotButtons = Array.from(document.querySelectorAll("[data-shot]"));
  const heroShot = document.getElementById("heroShot");

  const shots = {
    light: {
      src: "assets/dashboard-light.png",
      alt: "ECC Fan Controller 控制台浅色界面：温度卡片、历史图表、风扇曲线与预设"
    },
    dark: {
      src: "assets/dashboard-dark.png",
      alt: "ECC Fan Controller 控制台深色界面：温度卡片、历史图表、风扇曲线与预设"
    }
  };

  function selectShot(key) {
    const shot = shots[key];
    if (!shot || !heroShot) return;
    heroShot.src = shot.src;
    heroShot.alt = shot.alt;
    shotButtons.forEach(function (button) {
      const active = button.dataset.shot === key;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", active ? "true" : "false");
    });
  }

  function setTheme(theme) {
    root.dataset.theme = theme;
    try {
      localStorage.setItem("ecc-site-theme", theme);
    } catch (e) {}
    if (themeButton) {
      themeButton.textContent = theme === "dark" ? "亮色" : "暗色";
    }
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) {
      meta.setAttribute("content", theme === "dark" ? "#0f0f10" : "#fbfbf8");
    }
    selectShot(theme);
  }

  shotButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      selectShot(button.dataset.shot);
    });
  });

  if (themeButton) {
    themeButton.addEventListener("click", function () {
      setTheme(root.dataset.theme === "dark" ? "light" : "dark");
    });
  }

  setTheme(root.dataset.theme || "light");

  const year = document.getElementById("year");
  if (year) {
    year.textContent = String(new Date().getFullYear());
  }
})();

(function () {
  const root = document.documentElement;
  const toggle = document.getElementById("themeToggle");

  function setTheme(theme) {
    root.dataset.theme = theme;
    try {
      localStorage.setItem("ecc-site-theme", theme);
    } catch (e) {}
    if (toggle) {
      toggle.setAttribute("aria-label", theme === "dark" ? "切换浅色模式" : "切换深色模式");
    }
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) {
      meta.setAttribute("content", theme === "dark" ? "#050914" : "#4f46e5");
    }
  }

  if (toggle) {
    toggle.addEventListener("click", function () {
      const next = root.dataset.theme === "dark" ? "light" : "dark";
      setTheme(next);
      selectShot(next);
    });
  }

  const shots = {
    light: {
      src: "assets/dashboard-light.png",
      alt: "ECC Fan Controller 控制台浅色界面：CPU/GPU 温度、风扇曲线与历史图表"
    },
    dark: {
      src: "assets/dashboard-dark.png",
      alt: "ECC Fan Controller 控制台深色界面：CPU/GPU 温度、风扇曲线与历史图表"
    }
  };

  const heroShot = document.getElementById("heroShot");
  const shotLink = document.getElementById("shotLink");
  const tabs = Array.from(document.querySelectorAll(".shot-tab"));

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      const shot = shots[tab.dataset.shot];
      if (!shot || !heroShot) return;
      heroShot.src = shot.src;
      heroShot.alt = shot.alt;
      if (shotLink) shotLink.href = shot.src;
      tabs.forEach(function (item) {
        const active = item === tab;
        item.classList.toggle("active", active);
        item.setAttribute("aria-selected", active ? "true" : "false");
      });
    });
  });

  function selectShot(key) {
    const tab = tabs.find(function (item) {
      return item.dataset.shot === key;
    });
    if (tab) tab.click();
  }

  const initialTheme = root.dataset.theme || "light";
  setTheme(initialTheme);
  selectShot(initialTheme);

  const reveals = Array.from(document.querySelectorAll(".reveal"));
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  if ("IntersectionObserver" in window && !reduceMotion) {
    const observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            entry.target.classList.add("visible");
            observer.unobserve(entry.target);
          }
        });
      },
      { rootMargin: "0px 0px -8% 0px", threshold: 0.08 }
    );
    reveals.forEach(function (item) {
      observer.observe(item);
    });
  } else {
    reveals.forEach(function (item) {
      item.classList.add("visible");
    });
  }

  const year = document.getElementById("year");
  if (year) {
    year.textContent = String(new Date().getFullYear());
  }
})();

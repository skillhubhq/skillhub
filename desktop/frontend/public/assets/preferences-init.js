// 在首次绘制前应用主题；设置页和原生端共享同一偏好键。
(() => { let preference = "system"; try { preference = localStorage.getItem("skillhub.theme") || "system"; } catch {} const theme = preference === "light" || preference === "dark" ? preference : (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"); document.documentElement.dataset.theme = theme; })();

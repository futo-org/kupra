// Applies the saved theme before the page renders: @immich/ui's themeManager only applies it when it changes.
try {
  // themeManager stores its preference as JSON: "light", "dark" or "system".
  const preference = localStorage.getItem('immich-ui-theme');
  const dark =
    preference === '"dark"' || (preference !== '"light"' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.add(dark ? 'dark' : 'light');
} catch {
  // The default theme applies.
}

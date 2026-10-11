// Thème et animations réduites appliqués avant le premier rendu (pas de flash).
// Script classique bloquant chargé dans <head> : pas de script inline (CSP `script-src 'self'`).
;(function () {
  try {
    var pref = localStorage.getItem('occ-theme') || 'dark'
    var theme = pref === 'system' ? (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark') : pref
    document.documentElement.setAttribute('data-theme', theme)
    if (localStorage.getItem('occ-motion') === 'reduced') document.documentElement.setAttribute('data-motion', 'reduced')
  } catch (e) {}
})()

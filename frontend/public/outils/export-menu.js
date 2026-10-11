// Page « Exporter un menu » : prépare le favori. Script externe (CSP `script-src 'self'` : ni script ni onclick inline).
;(function () {
  var bm = document.getElementById('bm')
  // le bouton se glisse dans les favoris : un clic ici ne doit rien lancer
  bm.addEventListener('click', function (e) {
    e.preventDefault()
    alert('Glisse ce bouton dans ta barre de favoris, ne clique pas dessus ici.')
  })
  fetch('/outils/occ-menu-export.js', { cache: 'no-cache' })
    .then(function (r) { if (!r.ok) throw new Error(r.status); return r.text() })
    .then(function (code) {
      // retire les commentaires de bloc pour raccourcir le favori
      var compact = code.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\n\s*\/\/[^\n]*/g, '\n').replace(/\n\s+/g, '\n')
      bm.href = 'javascript:' + encodeURIComponent(compact)
      document.getElementById('bm-status').textContent = 'Prêt (' + Math.round(compact.length / 1024) + ' Ko).'
      document.getElementById('snippet').value = compact
    })
    .catch(function () { document.getElementById('bm-status').textContent = 'Impossible de charger le script.' })
  document.getElementById('copy').addEventListener('click', function (e) {
    navigator.clipboard.writeText(document.getElementById('snippet').value).then(function () { e.target.textContent = 'Copié ✓' })
  })
})()

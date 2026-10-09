/*
 * OCC DELIVERIES — « Exporter vers OCC »
 *
 * Bookmarklet / snippet console : lit les données structurées déjà présentes
 * dans la page d'un restaurant (Uber Eats, Takeaway/Just Eat, ou site du resto)
 * et produit un JSON au format d'import d'OCC (voir docs/ARCHITECTURE.md,
 * `RestaurantImport`). Aucun appel réseau, aucune connexion : uniquement le
 * contenu de la page que vous consultez.
 *
 * Sources, par ordre de préférence :
 *   1. JSON-LD schema.org (Restaurant / Menu / MenuSection / MenuItem)
 *   2. États applicatifs embarqués (<script type="application/json">,
 *      __NEXT_DATA__…) : recherche heuristique de sections { title, items[] }
 *      contenant des plats { title|name, price }.
 */
;(function (root) {
  'use strict'

  var PROVIDERS = [
    { id: 'ubereats', re: /(^|\.)ubereats\.com$/ },
    { id: 'takeaway', re: /(^|\.)(takeaway\.com|just-eat\.[a-z.]+|lieferando\.[a-z]+|thuisbezorgd\.nl)$/ },
  ]

  function slugify(s) {
    return String(s || '')
      .normalize('NFD')
      .replace(/[̀-ͯ]/g, '')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 60)
  }

  /** "12,50" | "12.50" | 12.5 | "€ 12,50" → 1250 ; centimes déjà entiers si `cents`. */
  function toCents(v, cents) {
    if (v == null || v === '') return 0
    if (typeof v === 'object') v = v.price != null ? v.price : v.amount != null ? v.amount : v.value
    if (typeof v === 'number') return cents ? Math.round(v) : Math.round(v * 100)
    var m = String(v).replace(/\s/g, '').match(/-?\d+(?:[.,]\d{1,2})?/)
    if (!m) return 0
    return Math.round(parseFloat(m[0].replace(',', '.')) * 100)
  }

  function asArray(x) {
    return x == null ? [] : Array.isArray(x) ? x : [x]
  }

  function hasType(o, t) {
    return o && asArray(o['@type']).some(function (x) { return String(x).toLowerCase() === t })
  }

  function clean(s, max) {
    return String(s || '').replace(/\s+/g, ' ').trim().slice(0, max || 500)
  }

  function guessEmoji(cuisines, name) {
    var s = (cuisines.join(' ') + ' ' + name).toLowerCase()
    var table = [
      [/pizz/, '🍕'], [/burger/, '🍔'], [/sushi|japon/, '🍣'], [/pok[eé]/, '🥗'], [/frit|snack/, '🍟'],
      [/kebab|turc|grec|pita/, '🥙'], [/liban|falafel/, '🧆'], [/indi/, '🍛'], [/tha[iï]|viet|asia|chin|wok/, '🍜'],
      [/tacos|mexic/, '🌮'], [/poulet|chicken/, '🍗'], [/salad|healthy|v[eé]g/, '🥗'], [/sandwich|bagel/, '🥪'],
      [/p[aâ]tes|pasta|ital/, '🍝'], [/dessert|glace|gaufre|cr[eê]pe/, '🧇'],
    ]
    for (var i = 0; i < table.length; i++) if (table[i][0].test(s)) return table[i][1]
    return '🍽️'
  }

  // ---------- 1. JSON-LD ----------
  function collectJsonLd(doc) {
    var out = []
    var nodes = doc.querySelectorAll('script[type="application/ld+json"]')
    for (var i = 0; i < nodes.length; i++) {
      try {
        var data = JSON.parse(nodes[i].textContent || 'null')
        asArray(data).forEach(function walk(o) {
          if (!o || typeof o !== 'object') return
          out.push(o)
          if (o['@graph']) asArray(o['@graph']).forEach(walk)
        })
      } catch (e) { /* JSON-LD invalide : ignoré */ }
    }
    return out
  }

  function itemFromLd(it) {
    var offer = asArray(it.offers)[0] || {}
    var price = toCents(offer.price != null ? offer.price : offer.lowPrice)
    if (!it.name || price <= 0) return null
    var tags = []
    asArray(it.suitableForDiet).forEach(function (d) {
      d = String(d)
      if (/Vegan/i.test(d)) tags.push('vegan')
      else if (/Vegetarian/i.test(d)) tags.push('veggie')
      else if (/GlutenFree/i.test(d)) tags.push('gluten_free')
    })
    return { name: clean(it.name, 120), description: clean(it.description, 400), price: price, tags: tags, option_groups: [], popular: false }
  }

  function fromJsonLd(doc) {
    var objs = collectJsonLd(doc)
    var r = objs.filter(function (o) {
      return hasType(o, 'restaurant') || hasType(o, 'foodestablishment') || hasType(o, 'localbusiness')
    })[0]
    var menus = objs.filter(function (o) { return hasType(o, 'menu') })
    if (!r && !menus.length) return null
    r = r || {}
    var categories = []
    asArray(r.hasMenu).concat(menus).forEach(function (menu) {
      if (!menu || typeof menu !== 'object') return
      asArray(menu.hasMenuSection).forEach(function (sec) {
        var items = asArray(sec.hasMenuItem).map(itemFromLd).filter(Boolean)
        if (items.length) categories.push({ name: clean(sec.name, 80) || 'Menu', items: items })
      })
      var loose = asArray(menu.hasMenuItem).map(itemFromLd).filter(Boolean)
      if (loose.length) categories.push({ name: 'Menu', items: loose })
    })
    var addr = r.address || {}
    if (typeof addr === 'string') addr = { streetAddress: addr }
    var geo = r.geo || {}
    var rating = r.aggregateRating || {}
    return {
      name: clean(r.name, 120),
      description: clean(r.description, 300),
      cuisines: asArray(r.servesCuisine).map(function (c) { return clean(c, 40).toLowerCase() }).filter(Boolean),
      address: [addr.streetAddress, [addr.postalCode, addr.addressLocality].filter(Boolean).join(' ')].filter(Boolean).join(', '),
      lat: parseFloat(geo.latitude) || 0,
      lng: parseFloat(geo.longitude) || 0,
      phone: clean(r.telephone, 30),
      rating: Math.round((parseFloat(rating.ratingValue) || 0) * 10) / 10,
      rating_count: parseInt(rating.reviewCount || rating.ratingCount, 10) || 0,
      price_level: typeof r.priceRange === 'string' ? Math.min(4, Math.max(0, (r.priceRange.match(/[€$]/g) || []).length)) : 0,
      categories: categories,
    }
  }

  // ---------- 2. États embarqués (heuristique) ----------
  function itemFromState(o) {
    var name = o.title || o.name
    if (typeof name !== 'string' || !name.trim()) return null
    var raw = o.price != null ? o.price : o.priceTagline != null ? o.priceTagline : o.prices
    if (raw == null) return null
    // Uber Eats : centimes entiers ; Takeaway : euros décimaux ou objet { delivery }
    var cents = typeof raw === 'number' && Number.isInteger(raw) && raw >= 100
    if (raw && typeof raw === 'object' && !Array.isArray(raw)) raw = raw.delivery != null ? raw.delivery : raw.deliveryPrice != null ? raw.deliveryPrice : raw.price
    var price = toCents(raw, cents)
    if (price <= 0 || price > 50000) return null
    return {
      name: clean(name, 120),
      description: clean(o.itemDescription || o.description, 400),
      price: price,
      tags: [],
      option_groups: [],
      popular: false,
    }
  }

  function fromEmbeddedState(doc) {
    var blobs = []
    var nodes = doc.querySelectorAll('script[type="application/json"], script#__NEXT_DATA__')
    for (var i = 0; i < nodes.length; i++) {
      var t = nodes[i].textContent || ''
      if (t.length < 50) continue
      try {
        var parsed = JSON.parse(t)
        // certains états sont doublement encodés
        if (typeof parsed === 'string') parsed = JSON.parse(parsed)
        blobs.push(parsed)
      } catch (e) { /* ignoré */ }
    }
    var categories = []
    var seen = {}
    var title = ''
    function visit(o, depth) {
      if (!o || typeof o !== 'object' || depth > 40) return
      if (Array.isArray(o)) { o.forEach(function (x) { visit(x, depth + 1) }); return }
      var listKey = ['itemList', 'items', 'products', 'menuItems', 'catalogItems'].filter(function (k) {
        return Array.isArray(o[k]) && o[k].length
      })[0]
      var secName = o.title || o.name
      if (listKey && typeof secName === 'string') {
        var items = o[listKey].map(function (x) { return x && typeof x === 'object' ? itemFromState(x) : null }).filter(Boolean)
        if (items.length >= 1 && items.length >= o[listKey].length / 2) {
          var key = secName + '|' + items.map(function (x) { return x.name }).join(',')
          if (!seen[key]) { seen[key] = 1; categories.push({ name: clean(secName, 80), items: items }) }
          return
        }
      }
      if (!title && typeof o.title === 'string' && (o.location || o.storeUuid || o.uuid) && o.location) title = o.title
      for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) visit(o[k], depth + 1)
    }
    blobs.forEach(function (b) { visit(b, 0) })
    if (!categories.length) return null
    return { name: title, categories: categories }
  }

  // ---------- assemblage ----------
  function extract(doc, href) {
    var url
    try { url = new URL(href) } catch (e) { url = { hostname: '', href: href } }
    var ld = fromJsonLd(doc) || {}
    var st = (!ld.categories || !ld.categories.length) ? fromEmbeddedState(doc) : null
    var categories = (ld.categories && ld.categories.length ? ld.categories : (st && st.categories) || [])
    var ogTitle = doc.querySelector('meta[property="og:title"]')
    var name = ld.name || (st && st.name) || clean(ogTitle && ogTitle.getAttribute('content'), 120) || clean(doc.title, 120)
    name = name.replace(/\s*[|–-]\s*(Uber Eats|Takeaway\.com|Just Eat).*$/i, '').replace(/^Commandez?\s+/i, '').trim()
    var cuisines = ld.cuisines || []
    var provider = PROVIDERS.filter(function (p) { return p.re.test(url.hostname) })[0]
    var canonical = doc.querySelector('link[rel="canonical"]')
    var pageUrl = (canonical && canonical.getAttribute('href')) || String(url.href).split('?')[0]
    var count = categories.reduce(function (n, c) { return n + c.items.length }, 0)
    return {
      restaurant: {
        slug: slugify(name),
        name: name,
        description: ld.description || '',
        emoji: guessEmoji(cuisines, name),
        cover_url: '',
        cuisines: cuisines,
        address: ld.address || '',
        lat: ld.lat || 0,
        lng: ld.lng || 0,
        phone: ld.phone || '',
        rating: ld.rating || 0,
        rating_count: ld.rating_count || 0,
        price_level: ld.price_level || 2,
        eta_min: 25,
        eta_max: 45,
        delivery_fee: 0,
        min_order: 0,
        providers: provider ? [{ id: provider.id, url: pageUrl }] : [],
        active: true,
        categories: categories,
        source_urls: [pageUrl],
        menu_checked_at: new Date().toISOString().slice(0, 10),
      },
      stats: { categories: categories.length, items: count, source: ld.categories && ld.categories.length ? 'json-ld' : st ? 'état embarqué' : 'aucune' },
    }
  }

  // ---------- interface (navigateur) ----------
  function run(win) {
    var doc = win.document
    var res = extract(doc, win.location.href)
    var r = res.restaurant
    var json = JSON.stringify(r, null, 2)
    var old = doc.getElementById('occ-export')
    if (old) old.remove()
    var box = doc.createElement('div')
    box.id = 'occ-export'
    box.setAttribute('style', 'position:fixed;z-index:2147483647;right:16px;bottom:16px;width:340px;max-width:calc(100vw - 32px);' +
      'background:#121217;color:#F6F4EF;border:1px solid rgba(255,255,255,.12);border-radius:20px;padding:16px;' +
      'font:14px/1.4 system-ui,sans-serif;box-shadow:0 12px 40px rgba(0,0,0,.5)')
    var ok = res.stats.items > 0
    box.innerHTML =
      '<div style="font-weight:700;font-size:16px;margin-bottom:4px">🔥 Exporter vers OCC</div>' +
      '<div style="color:#A3A1AB;margin-bottom:12px">' +
      (ok ? '<b style="color:#F6F4EF"></b><br>' + res.stats.categories + ' catégories · ' + res.stats.items + ' plats · source : ' + res.stats.source
          : 'Aucun menu détecté sur cette page. Ouvrez la page d’un restaurant (pas la liste) et attendez qu’elle soit chargée.') +
      (ok && (!r.lat || !r.address) ? '<br><span style="color:#F5B83D">Adresse/coordonnées à compléter dans /admin.</span>' : '') +
      '</div>' +
      '<div style="display:flex;gap:8px;flex-wrap:wrap">' +
      (ok ? '<button data-a="dl" style="flex:1;padding:10px;border-radius:12px;border:0;background:linear-gradient(135deg,#FF6A3D,#FFB547);color:#1A0B05;font-weight:700;cursor:pointer">Télécharger .json</button>' +
            '<button data-a="cp" style="flex:1;padding:10px;border-radius:12px;border:1px solid rgba(255,255,255,.16);background:#1A1A21;color:#F6F4EF;cursor:pointer">Copier</button>' : '') +
      '<button data-a="x" style="padding:10px 12px;border-radius:12px;border:1px solid rgba(255,255,255,.16);background:transparent;color:#A3A1AB;cursor:pointer">Fermer</button></div>'
    if (ok) box.querySelector('b').textContent = r.name
    box.addEventListener('click', function (e) {
      var a = e.target && e.target.getAttribute && e.target.getAttribute('data-a')
      if (a === 'x') box.remove()
      if (a === 'cp' && win.navigator.clipboard) win.navigator.clipboard.writeText(json).then(function () { e.target.textContent = 'Copié ✓' })
      if (a === 'dl') {
        var blob = new win.Blob([json], { type: 'application/json' })
        var link = doc.createElement('a')
        link.href = win.URL.createObjectURL(blob)
        link.download = 'occ-' + (r.slug || 'restaurant') + '.json'
        doc.body.appendChild(link); link.click(); link.remove()
      }
    })
    doc.body.appendChild(box)
    return res
  }

  var api = { extract: extract, toCents: toCents, slugify: slugify, run: run }
  if (typeof module === 'object' && module.exports) module.exports = api
  root.OCCMenuExport = api
  if (typeof window !== 'undefined' && !root.__OCC_EXPORT_NO_RUN__) run(window)
})(typeof globalThis !== 'undefined' ? globalThis : this)

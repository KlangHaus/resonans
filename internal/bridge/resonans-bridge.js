/*
 * Resonans visual-editing bridge (protocol v1).
 *
 * Include it on the frontend's PREVIEW route only:
 *
 *   <script src="https://<resonans>/bridge/v1/resonans-bridge.js"></script>
 *   ResonansBridge.init({
 *     studioOrigin: 'https://studio.example',   // exact origin, no path
 *     onUpdate: function (envelope) { render(envelope) },
 *     accent: '#0059b3',                        // optional frame color
 *   })
 *
 * and mark editable elements with the path of the value they render, using the
 * delivery envelope's own shape: data-resonans-path="data.title" or
 * data-resonans-path="components.2.heading".
 *
 * Security model: messages are accepted ONLY from window.parent AND only when
 * event.origin equals studioOrigin exactly; messages are posted ONLY to
 * studioOrigin, never '*'. The bridge does nothing outside an iframe.
 */
(function (global) {
  'use strict'

  var VERSION = 1
  var FROM_STUDIO = 'resonans-studio'
  var FROM_BRIDGE = 'resonans-bridge'
  var ORIGIN_RE = /^https?:\/\/[^/?#\s]+$/
  var PATH_RE = /^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+){0,19}$/
  var ATTR = 'data-resonans-path'

  function inactive(reason) {
    return { active: false, reason: reason, destroy: function () {} }
  }

  function init(opts) {
    opts = opts || {}
    var studioOrigin = opts.studioOrigin
    if (typeof studioOrigin !== 'string' || !ORIGIN_RE.test(studioOrigin)) {
      return inactive('studioOrigin must be an exact origin like https://studio.example')
    }
    if (global.parent === global) {
      return inactive('not inside an iframe')
    }
    var doc = global.document

    function post(type, payload) {
      var msg = { source: FROM_BRIDGE, v: VERSION, type: type }
      for (var k in payload) {
        if (Object.prototype.hasOwnProperty.call(payload, k)) msg[k] = payload[k]
      }
      global.parent.postMessage(msg, studioOrigin)
    }

    var accent = typeof opts.accent === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(opts.accent) ? opts.accent : '#0059b3'

    // Two fixed overlays that never intercept pointers:
    //   hover    — "this can be edited": thin frame + the value's path
    //   selected — "this is what you are editing": solid frame + "✎ Valgt blok"
    // They are independent, so hovering elsewhere never drops the selection.
    function overlay(css, labelCss) {
      var box = doc.createElement('div')
      box.setAttribute('data-resonans-overlay', '')
      box.style.cssText =
        'position:fixed;z-index:2147483646;pointer-events:none;display:none;box-sizing:border-box;' + css
      var label = doc.createElement('span')
      label.style.cssText =
        'position:absolute;white-space:nowrap;font:500 10px/16px system-ui,sans-serif;padding:0 7px;' +
        'border-radius:5px;color:#fff;background:' + accent + ';' + labelCss
      box.appendChild(label)
      doc.body.appendChild(box)
      // The PATH is tracked, not the element: a site re-renders on every update
      // and replaces the element, so the frame is re-resolved by path.
      return { box: box, label: label, el: null, path: null }
    }
    var hover = overlay('border:1px solid ' + accent + ';border-radius:4px;opacity:.6', 'left:-1px;top:-17px;opacity:.9')
    var selected = overlay(
      'outline:2px solid ' + accent + ';outline-offset:3px;border-radius:8px;z-index:2147483647',
      'left:8px;top:-11px',
    )

    function byPath(path) {
      return path && PATH_RE.test(path) ? doc.querySelector('[' + ATTR + '="' + path + '"]') : null
    }

    function place(o, el, text) {
      o.el = el
      if (!el) {
        o.box.style.display = 'none'
        return
      }
      var r = el.getBoundingClientRect()
      o.box.style.display = 'block'
      o.box.style.left = r.left + 'px'
      o.box.style.top = r.top + 'px'
      o.box.style.width = r.width + 'px'
      o.box.style.height = r.height + 'px'
      o.label.textContent = text
    }

    function setHover(el) {
      hover.path = el ? el.getAttribute(ATTR) : null
      // No hover frame on top of the selected element: the selection says more.
      place(hover, el && el !== selected.el ? el : null, hover.path)
    }

    function setSelected(path) {
      selected.path = path && PATH_RE.test(path) ? path : null
      place(selected, byPath(selected.path), '✎ Valgt blok')
    }

    function target(node) {
      var el = node && node.nodeType === 1 ? node : node && node.parentElement
      el = el && el.closest ? el.closest('[' + ATTR + ']') : null
      return el && PATH_RE.test(el.getAttribute(ATTR)) ? el : null
    }

    function onMove(e) {
      var el = target(e.target)
      if (el !== hover.el) setHover(el)
    }

    function onReflow() {
      if (selected.path) place(selected, byPath(selected.path), '✎ Valgt blok')
      if (hover.path) {
        var h = byPath(hover.path)
        place(hover, h && h !== selected.el ? h : null, hover.path)
      }
    }

    // Capture phase: in the editor a click selects the field; it must not
    // follow a link or submit a form inside the preview.
    function onClick(e) {
      var el = target(e.target)
      if (!el) return
      e.preventDefault()
      e.stopPropagation()
      var path = el.getAttribute(ATTR)
      setSelected(path)
      setHover(null)
      post('select', { path: path })
    }

    function onMessage(e) {
      if (e.source !== global.parent || e.origin !== studioOrigin) return
      var d = e.data
      if (!d || d.source !== FROM_STUDIO || d.v !== VERSION) return
      if (d.type === 'update' && d.envelope && typeof opts.onUpdate === 'function') {
        opts.onUpdate(d.envelope)
        global.requestAnimationFrame(onReflow)
      } else if (d.type === 'select') {
        setSelected(typeof d.path === 'string' ? d.path : null)
        if (selected.el && selected.el.scrollIntoView) {
          selected.el.scrollIntoView({ block: 'center', behavior: 'smooth' })
        }
      }
    }

    global.addEventListener('message', onMessage)
    doc.addEventListener('mousemove', onMove, true)
    doc.addEventListener('click', onClick, true)
    global.addEventListener('scroll', onReflow, true)
    global.addEventListener('resize', onReflow)
    post('ready', { href: String(global.location.href) })

    return {
      active: true,
      destroy: function () {
        global.removeEventListener('message', onMessage)
        doc.removeEventListener('mousemove', onMove, true)
        doc.removeEventListener('click', onClick, true)
        global.removeEventListener('scroll', onReflow, true)
        global.removeEventListener('resize', onReflow)
        ;[hover.box, selected.box].forEach(function (b) {
          if (b.parentNode) b.parentNode.removeChild(b)
        })
      },
    }
  }

  // attr(path) -> { 'data-resonans-path': path } for spreading onto elements.
  function attr(path) {
    var o = {}
    o[ATTR] = path
    return o
  }

  global.ResonansBridge = { version: VERSION, init: init, attr: attr }
})(window)

// The account row in app.vue: with login on, it is the account button and never
// the development-mode label. A `v-else` pairs with the nearest preceding element
// that has `v-if` or `v-else-if`, so an element put between the two silently
// becomes the other half of the chain.
//
// No Vue renderer is available to these tests, so this reads the template with
// jsdom's HTML parser and applies Vue's chain rule to it: within one parent, a
// chain is a `v-if` element followed by `v-else-if`/`v-else` elements, with only
// whitespace and comments between them, and the first branch whose condition holds
// is the one rendered.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(join(__dirname, 'app.vue'), 'utf8')

function templateRoot(): DocumentFragment {
  const start = source.indexOf('<template>')
  const end = source.lastIndexOf('</template>')
  const holder = document.createElement('template')
  holder.innerHTML = source.slice(start + '<template>'.length, end)
  return holder.content
}

/** The elements that render under `state`, which maps each condition expression
 *  used in a v-if chain to its value. An unknown expression fails the test, so a
 *  new condition cannot pass by being ignored. */
function rendered(root: ParentNode, state: Record<string, boolean>): Element[] {
  const value = (expr: string): boolean => {
    const v = state[expr]
    if (v === undefined) throw new Error(`no value given for the condition ${expr}`)
    return v
  }
  const out: Element[] = []
  let chainOpen = false
  let chainTaken = false
  for (const el of Array.from(root.children)) {
    let show: boolean
    if (el.hasAttribute('v-if')) {
      chainOpen = true
      chainTaken = show = value(el.getAttribute('v-if') ?? '')
    } else if (el.hasAttribute('v-else-if') || el.hasAttribute('v-else')) {
      if (!chainOpen) throw new Error(`a ${el.tagName} with v-else has no v-if before it`)
      show = !chainTaken && (el.hasAttribute('v-else') || value(el.getAttribute('v-else-if') ?? ''))
      chainTaken ||= show
      if (el.hasAttribute('v-else')) chainOpen = false
    } else {
      chainOpen = false
      show = true
    }
    if (show) out.push(el, ...rendered(el, state))
  }
  return out
}

const accountRail = () => {
  const button = Array.from(templateRoot().querySelectorAll('button')).find((b) => b.getAttribute('@click') === 'logOut')
  if (!button?.parentElement) throw new Error('the logout button is not in app.vue')
  return button.parentElement
}
// Conditions in the rail that are not about the account. Fixed, so a new one
// fails the test until someone decides what it should be here.
const others = { showHelp: false }
const texts = (els: Element[]) => els.map((el) => el.textContent ?? '').join(' ')
const devLabel = "t('My account (development mode, no login)')"
const devText = "t('My account')"

describe('the account row', () => {
  it.each([false, true])('with login on (logout failed: %s) shows the button and no development label', (logoutFailed) => {
    const out = rendered(accountRail(), { ...others, 'auth.enabled.value': true, logoutFailed })
    expect(out.some((el) => el.getAttribute('@click') === 'logOut')).toBe(true)
    expect(out.some((el) => el.getAttribute(':title') === devLabel)).toBe(false)
    expect(texts(out)).not.toContain(devText)
  })

  it('shows the failed logout only when it failed', () => {
    const say = "t('Logout failed. You are still logged in.')"
    expect(texts(rendered(accountRail(), { ...others, 'auth.enabled.value': true, logoutFailed: false }))).not.toContain(say)
    expect(texts(rendered(accountRail(), { ...others, 'auth.enabled.value': true, logoutFailed: true }))).toContain(say)
  })

  // The control that shows the cells above can see the label at all.
  it('POSITIVE CONTROL: with login off, shows the development label and no button', () => {
    const out = rendered(accountRail(), { ...others, 'auth.enabled.value': false, logoutFailed: false })
    expect(out.some((el) => el.getAttribute(':title') === devLabel)).toBe(true)
    expect(out.some((el) => el.getAttribute('@click') === 'logOut')).toBe(false)
  })
})

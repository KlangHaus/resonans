// Site chrome: the few strings the header and footer need on every page. The
// pages themselves come from Resonans; these do not, because they are the frame
// around the content rather than content (klanghaus.dk keeps them in copy.ts).
export const SITE = {
  name: 'KlangHaus',
  logo: '/klanghaus-logo.svg',
  mail: 'mail@klanghaus.dk',
  // Company identification (CVR address, 20 — not the studios' 20H).
  legal: 'Asp Productions ApS · CVR 36708077 · Raffinaderivej 20, 2300 København S',
}

/** The building's floor plan, as on klanghaus.dk: which studio sits where. It is
 *  physical layout, not content — sizes and availability come from Resonans. */
export const PLAN_TOP = [5, 4, 3, 2, 1]
export const PLAN_BOTTOM = [9, 8, 7, 6, 10]

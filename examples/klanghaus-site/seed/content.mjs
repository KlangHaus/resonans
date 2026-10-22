// The pages this example seeds into Resonans. The front page and the privacy
// policy are klanghaus.dk's own text, moved unchanged; the bunker page is TEST
// CONTENT written for this example and says so on the page.
//
// `img('name')` is replaced by the uploaded asset id (seed.mjs uploads
// seed/images/<name>.jpg first). i18n fields are { da: '…' }.
import { readFileSync } from 'node:fs'

const da = (s) => ({ da: s })
const b = (type, fields) => ({ _type: type, ...fields })
export const img = (name) => ({ __img: name })

const MAIL = 'mail@klanghaus.dk'
const WAITLIST_URL = 'https://forms.clickup.com/90122143481/f/2kxv5hqt-732/JU53YTR3DCQXV9JFHT'

const studio = (nr, m2, availableFrom = '') =>
  ({ title: da(`Studie ${nr}`), body: da(`${m2} m²`), icon: availableFrom })

const imagesRow = (...names) =>
  b('columns', { columns: names.map(([name, alt]) => ({ content: [b('image', { asset: img(name), alt: da(alt) })] })) })

export const pages = [
  {
    slug: 'forside',
    title: 'Forside',
    body: {
      seoTitle: da('KlangHaus — lydstudier til leje i København'),
      seoDescription: da(
        'Ti lydregulerede studier til leje på Amager, København S — dit eget rum på løbende lejemål, ikke et øvelokale du booker i timer. Musikerdrevet siden 2015.',
      ),
      body: [
        b('hero', {
          eyebrow: da('Lydstudier til leje · Raffinaderivej, København S · Amager'),
          heading: da('KlangHaus er ikke et studie.|Det er *et hjem for musikere.*'),
          subheading: da(
            'Ti lydregulerede studier i ét hus, drevet af musikere siden 2015. Dit eget faste rum — og et hus fuld af folk der laver det samme som dig.',
          ),
          image: img('studie-lydisolering-1000'),
          imageAlt: da('Tomt lydstudie hos KlangHaus med synlig akustik-lamelvæg og vindue'),
          ctaLabel: da('Hør om ledige studier →'),
          ctaHref: '#kontakt',
          secondaryCtaLabel: da('Se studierne'),
          secondaryCtaHref: '#studierne',
        }),
        b('feature_grid', {
          variant: 'facts',
          features: [
            { title: da('10'), body: da('lydregulerede studier, 5–17 m²') },
            { title: da('Fiber'), body: da('i hele huset') },
            { title: da('2015'), body: da('drevet af musikere siden') },
            { title: da('Dit'), body: da('eget rum — ikke en booking') },
          ],
        }),

        b('section_header', {
          anchor: 'studierne',
          eyebrow: da('Studierne'),
          heading: da('Et rum der er dit — i et hus bygget til lyd'),
          lead: da(
            'Studierne er lydregulerede, så flere kan arbejde samtidig i huset. Det er ikke et øvelokale du booker i timer — det er dit eget rum, på løbende lejemål.',
          ),
        }),
        b('feature_grid', {
          variant: 'cards',
          features: [
            {
              icon: '◉',
              title: da('Lydreguleret'),
              body: da(
                'Studierne er lydbehandlet og adskilt fra hinanden. Lydregulering er ikke det samme som total stilhed — hvor meget der høres mellem rum, afhænger af rummet og hvad du spiller.',
              ),
            },
            {
              icon: '⌂',
              title: da('Dit eget rum, 5–17 m²'),
              body: da('Fra det lille mix-rum til det store produktionsrum. Et fast rum på løbende lejemål — ikke en booking pr. time.'),
            },
            {
              icon: '↯',
              title: da('Fiber og alarm'),
              body: da(
                'Symmetrisk fiberforbindelse i hele huset til store sessions-overførsler. Alarmsystem og adgangsstyring, så gearet står sikkert.',
              ),
            },
            {
              icon: '☕',
              title: da('Fælles faciliteter'),
              body: da(
                'Te-køkken, toilet og husregler der faktisk virker — fordi de er lavet af folk der selv har siddet i et studie. Vi sørger for god kaffe fra Blackbird Coffee. Du kan også booke bunkeren.',
              ),
            },
          ],
        }),
        imagesRow(
          ['indrettet-studie-a-1200', 'Indrettet studie hos en lejer med guitarer på væggen, kontrabas, synthesizere og kulørt lys'],
          ['indrettet-studie-b-480', 'Indrettet produktionsstudie hos en lejer med studiemonitorer, keyboard og guitarer'],
        ),

        b('section_header', {
          anchor: 'oversigt',
          eyebrow: da('Studieoversigt'),
          navLabel: da('Oversigt'),
          heading: da('Ti studier — find dit'),
        }),
        // Plan: one feature per studio. icon = ISO date the studio is free from.
        b('feature_grid', {
          variant: 'plan',
          features: [
            studio(1, 17), studio(2, 15), studio(3, 15, '2026-11-01'), studio(4, 15), studio(5, 10),
            studio(6, 5), studio(7, 5), studio(8, 10), studio(9, 15), studio(10, 10),
          ],
        }),

        b('section_header', {
          anchor: 'huset',
          variant: 'band',
          eyebrow: da('Huset'),
          heading: da('Et kontorfællesskab — bare for folk der laver lyd'),
          lead: da(
            'Du kender modellen fra kontorfællesskaber: eget rum, fælles hus, folk omkring dig der forstår hvad du laver. Hos os er de folk musikere og producere — og det er pointen.',
          ),
        }),
        imagesRow(
          ['facade-gaardareal-1200', 'Facaden på Raffinaderivej i solskin — rød murstensbygning med gårdareal og bordbænkesæt'],
          ['faelleskoekken-1200', "Fælleskøkkenet med KlangHaus' logo i dørglasset og åben dør til haven"],
        ),
        b('quote', {
          quote: da('Vi er bevidst ikke en korporat udlejningsvirksomhed. Vi er en del af det miljø vi lejer ud til.'),
          author: "Fra KlangHaus' manifest",
        }),
        b('feature_grid', {
          variant: 'checklist',
          features: [
            {
              title: da('Drevet af musikere.'),
              body: da(
                'KlangHaus er en familievirksomhed startet i 2015 af to brødre, begge tidligere musikere og producere. Du taler med dem der driver huset — ikke en ejendomsadministration.',
              ),
            },
            {
              title: da('Mange musikere under ét tag.'),
              body: da('Mix-folk, producere, sangskrivere, lyddesignere. Naboen kan være den næste du laver noget med.'),
            },
            {
              title: da('Et hus, ikke en udlejningsportal.'),
              body: da('Fælles køkken, fælles beskeder når der sker noget i huset, og en dør du kan banke på.'),
            },
            {
              title: da('Siden 2015.'),
              body: da('Samme ejere, samme selskab, drevet af musikere hele vejen. Det er et sted man bliver.'),
            },
          ],
        }),

        b('section_header', {
          anchor: 'saadan',
          eyebrow: da('Sådan lejer du'),
          heading: da('Tre skridt — enkelt og forudsigeligt'),
          lead: da('Vi holder det enkelt og forudsigeligt, for begge parter.'),
        }),
        b('feature_grid', {
          variant: 'steps',
          features: [
            {
              title: da('Skriv til os'),
              body: da(
                'Fortæl kort hvad du laver og hvor meget plads du har brug for. Vi svarer personligt og fortæller hvad der er ledigt — eller hvornår noget bliver det.',
              ),
            },
            {
              title: da('Kom og se huset'),
              body: da('Du ser rummene, møder dem der er her, og mærker om det passer. Det gør man bedst med sine egne ører.'),
            },
            {
              title: da('Løbende lejemål'),
              body: da(
                'Forudbetalt leje og depositum er vores standardvilkår. Størrelsen af begge, prisen og alle øvrige omkostninger står i lejeaftalen for netop det rum, du vælger — skriftligt, før du siger ja.',
              ),
            },
          ],
        }),

        b('section_header', {
          anchor: 'venteliste',
          variant: 'waitlist',
          eyebrow: da('Venteliste'),
          heading: da('Skriv dig på ventelisten'),
          lead: da('Er der ikke et ledigt studie i din størrelse lige nu, kan du skrive dig på ventelisten.'),
        }),
        b('cta', { label: da('Skriv dig på ventelisten ↗'), href: WAITLIST_URL }),
        b('rich_text', {
          format: 'html',
          value: da(
            'Formularen åbner hos ClickUp i et nyt vindue. <a href="/privatlivspolitik#venteliste">Sådan behandler vi dine oplysninger</a>',
          ),
        }),

        b('section_header', {
          anchor: 'kontakt',
          variant: 'contact',
          eyebrow: da('Kontakt'),
          heading: da('Mangler du et rum at lave musik i?'),
          lead: da(
            'Skriv til os. Vi svarer personligt — du taler direkte med dem der driver huset. Fortæl hvad du laver, hvor meget plads du har brug for, og hvornår du gerne vil i gang.',
          ),
        }),
        b('cta', { label: da('Hør om ledige studier →'), href: `mailto:${MAIL}?subject=Ledige%20studier%20hos%20KlangHaus` }),
        b('rich_text', {
          format: 'html',
          value: da(
            `<b>Kontakt</b><a href="mailto:${MAIL}">${MAIL}</a><br />` +
              '<b>Instagram</b><a href="https://www.instagram.com/klanghaus.dk/" target="_blank" rel="noopener">@klanghaus.dk</a><br />' +
              '<b>Adresse</b>Raffinaderivej 20H, 2300 København S (Amager)<br />' +
              '<b>Selskab</b>Asp Productions ApS · CVR 36708077',
          ),
        }),
      ],
    },
  },

  {
    slug: 'privatlivspolitik',
    title: 'Privatlivspolitik',
    body: {
      seoTitle: da('Privatlivspolitik — KlangHaus'),
      seoDescription: da('Privatlivspolitik for klanghaus.dk — hvad vi behandler når du besøger siden eller skriver til os.'),
      body: [
        // This test site runs scripts and a server; the policy's own premises
        // (no scripts, no cookies) describe klanghaus.dk, not this copy.
        b('callout', {
          variant: 'warn',
          value: da(
            '<p><strong>Testkopi.</strong> Denne tekst er kopieret fra klanghaus.dk til et testprojekt og gælder ikke her. Den gældende privatlivspolitik står på <a href="https://klanghaus.dk/privatlivspolitik/">klanghaus.dk</a>.</p>',
          ),
        }),
        b('rich_text', { format: 'html', value: da(readFileSync(new URL('./privatlivspolitik.html', import.meta.url), 'utf8')) }),
      ],
    },
  },

  {
    slug: 'bunker-udlejning',
    title: 'Bunker-udlejning (testside)',
    body: {
      seoTitle: da('Bunkeren — testside — KlangHaus'),
      seoDescription: da('Testside i Resonans-eksemplet. Ikke et tilbud.'),
      body: [
        b('hero', {
          eyebrow: da('Testside · eksempelindhold i Resonans'),
          heading: da('Bunkeren|*en landingsside til test.*'),
          subheading: da(
            'Denne side er skrevet til Resonans-eksemplet for at vise en landingsside ved siden af forsiden. Den beskriver ikke et rigtigt tilbud, og intet her er en pris, en dato eller et løfte.',
          ),
          ctaLabel: da('Se studierne →'),
          ctaHref: '/#studierne',
          secondaryCtaLabel: da('Til forsiden'),
          secondaryCtaHref: '/',
        }),
        b('section_header', {
          anchor: 'hvad',
          eyebrow: da('Eksempel'),
          heading: da('Sådan kunne en udlejningsside se ud'),
          lead: da('Kortene nedenfor viser layoutet. Teksten er pladsholder, så siden kan bruges til at afprøve redigering i Resonans.'),
        }),
        b('feature_grid', {
          variant: 'cards',
          features: [
            { icon: '◉', title: da('Kort ét'), body: da('Pladsholdertekst til et kort. Ret den i Resonans og se siden skifte ved næste visning.') },
            { icon: '⌂', title: da('Kort to'), body: da('Kortene er en feature_grid med variant "cards" — samme blok som forsidens.') },
            { icon: '↯', title: da('Kort tre'), body: da('Tilføj, fjern eller flyt kort i editoren; layoutet følger med.') },
          ],
        }),
        b('section_header', {
          anchor: 'trin',
          eyebrow: da('Eksempel'),
          heading: da('Tre trin som pladsholder'),
        }),
        b('feature_grid', {
          variant: 'steps',
          features: [
            { title: da('Første trin'), body: da('Trinene nummereres automatisk.') },
            { title: da('Andet trin'), body: da('Rækkefølgen er rækkefølgen i Resonans.') },
            { title: da('Tredje trin'), body: da('Variant "steps" på en feature_grid.') },
          ],
        }),
        b('section_header', {
          anchor: 'kontakt',
          variant: 'contact',
          heading: da('Spørgsmål til testsiden?'),
          lead: da('Kontaktfeltet er det samme som forsidens og hentes også fra Resonans.'),
        }),
        b('cta', { label: da('Til forsiden →'), href: '/' }),
        b('rich_text', { format: 'html', value: da('<b>Testindhold</b>Ikke et tilbud — en side til at afprøve Resonans.') }),
      ],
    },
  },
]

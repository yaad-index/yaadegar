import type { Config } from 'tailwindcss';

/*
 * The Yaadegar design system (#199). Colours and typeface families resolve
 * through the CSS custom properties defined in src/app.css (the measured source
 * of truth); this file gives each an ergonomic utility name.
 *
 * Colour token -> utility namespace mapping (CSS var names kept verbatim from
 * the measured set; the Tailwind keys below are the local, collision-free names):
 *
 *   --bg              -> page            (bg-page)
 *   --surface{,-alt,-accent} -> surface{,.alt,.accent}
 *   --border{,-subtle}       -> line{,.subtle}     (border-line, border-line-subtle)
 *   --divider                -> divider            (border-divider)
 *   --primary{,-hover,-tint} -> primary{,.hover,.tint}
 *   --accent-gold{,-tint}    -> gold{,.tint}
 *   --accent-green{,-tint}   -> green{,.tint}
 *   --text{,-heading,-muted} -> ink{,.heading,.muted}
 *
 * Spacing is deliberately NOT re-declared: Tailwind's default 4px-based scale
 * already is the design's 4/8/12/16/24/32/48 rhythm (utilities 1/2/3/4/6/8/12).
 */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			colors: {
				page: 'var(--bg)',
				surface: {
					DEFAULT: 'var(--surface)',
					alt: 'var(--surface-alt)',
					accent: 'var(--surface-accent)'
				},
				line: {
					DEFAULT: 'var(--border)',
					subtle: 'var(--border-subtle)'
				},
				divider: 'var(--divider)',
				primary: {
					DEFAULT: 'var(--primary)',
					hover: 'var(--primary-hover)',
					tint: 'var(--primary-tint)'
				},
				gold: {
					DEFAULT: 'var(--accent-gold)',
					tint: 'var(--accent-gold-tint)'
				},
				green: {
					DEFAULT: 'var(--accent-green)',
					tint: 'var(--accent-green-tint)'
				},
				ink: {
					DEFAULT: 'var(--text)',
					heading: 'var(--text-heading)',
					muted: 'var(--text-muted)'
				}
			},
			fontFamily: {
				display: 'var(--font-display)',
				ui: 'var(--font-ui)'
			},
			// Type scale 12/14/16/20/24/30/36, named by role. Sizes are the
			// suggested (declared) sizes from the measured set; the 24 step is
			// interpolated — no element landed on it — so treat it as a gap-filler.
			// Line-heights are an inference (only ink extents were measured), kept
			// restrained.
			fontSize: {
				chip: ['12px', '1.4'],
				ui: ['14px', '1.4'],
				body: ['16px', '1.6'],
				title: ['20px', '1.3'],
				// display-sm and display below are declared but UNVERIFIED — no element in
				// the set lands on either, so neither is a measurement (title and panel are).
				'display-sm': ['24px', '1.25'],
				// Display heading ("Your lists"/"Sign in"): 40px is the design value itself,
				// not a cap-height derivation — a calc() would only hide it. Measured as a
				// 28px cap at a 0.700 ratio (28 / 0.700 = 40.0), cross-checked by the same
				// procedure returning 20.0px on the three title elements. See #218.
				panel: ['40px', '1.2'],
				display: ['36px', '1.15'] // unverified (see the display-sm note)
			},
			borderRadius: {
				card: '12px' // list card / control radius
			},
			maxWidth: {
				content: '800px' // centred content column
			},
			minHeight: {
				card: '104px' // list card height
			},
			// Measured: the list card and welcome banner are separated from the page
			// by a soft downward shadow, not a border — ~3% darkening at the edge,
			// 12-14px falloff, almost nothing above (positive y-offset + negative
			// spread pull it downward). This is the card's separation; the 1px
			// --border outline belongs to the auth panel.
			boxShadow: {
				card: '0 6px 14px -2px rgb(46 40 36 / 0.06)'
			}
		}
	},
	plugins: []
} satisfies Config;

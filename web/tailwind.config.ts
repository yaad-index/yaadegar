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
				'display-sm': ['24px', '1.25'],
				panel: ['30px', '1.2'],
				display: ['36px', '1.15']
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
			// The one value not in the measured set: the design edges cards with a
			// 1px border, not a shadow, so this is a restrained inference kept
			// available for surfaces that need lift. Flagged for review.
			boxShadow: {
				card: '0 1px 2px rgb(46 40 36 / 0.04)'
			}
		}
	},
	plugins: []
} satisfies Config;

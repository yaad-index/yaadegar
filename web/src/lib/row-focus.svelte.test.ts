import { describe, it, expect, beforeEach } from 'vitest';
import { neighbourIds, restoreRowFocus } from './row-focus';

// #428: a row's own Delete / Archive / Put back button removes the row, the button
// goes with it, and focus fell to <body>. Named .svelte.test so it runs under jsdom:
// the behaviour is all about document.activeElement.

describe('neighbourIds', () => {
	it('tries the row that takes its place first, then the one before it', () => {
		expect(neighbourIds(['a', 'b', 'c'], 'b')).toEqual(['c', 'a']);
	});
	it('has only the row before it for the last row, only the row after for the first', () => {
		expect(neighbourIds(['a', 'b', 'c'], 'c')).toEqual(['b']);
		expect(neighbourIds(['a', 'b', 'c'], 'a')).toEqual(['b']);
	});
	it('has nothing for the only row, or for an id that is not in the group', () => {
		expect(neighbourIds(['a'], 'a')).toEqual([]);
		expect(neighbourIds(['a', 'b'], 'x')).toEqual([]);
	});
});

describe('restoreRowFocus', () => {
	let heading: HTMLElement;

	// Two rows shaped like the owner page's: the live row leads with a link, the
	// archived one with a form whose hidden input comes before its button.
	beforeEach(() => {
		document.body.innerHTML = `
			<h2 id="heading" tabindex="-1">Your items</h2>
			<ul>
				<li data-row-id="a"><a href="/x">Item A</a><button>Edit</button></li>
				<li data-row-id="b">
					<form><input type="hidden" name="item_id" value="b" /><button>Put back</button></form>
				</li>
			</ul>`;
		heading = document.getElementById('heading')!;
		(document.activeElement as HTMLElement | null)?.blur();
	});

	// The removed row's button, already detached, which is the state the page is in
	// once the reload has re-rendered the group.
	const gone = () => document.createElement('button');

	it("focuses the first candidate row's first control", () => {
		restoreRowFocus(document, gone(), ['a', 'b'], [heading]);
		expect(document.activeElement?.textContent).toBe('Item A');
	});

	it('moves on to the next candidate when a row is no longer there', () => {
		restoreRowFocus(document, gone(), ['zz', 'a'], [heading]);
		expect(document.activeElement?.textContent).toBe('Item A');
	});

	it('skips a hidden input, which is first in the row but cannot take focus', () => {
		restoreRowFocus(document, gone(), ['b'], [heading]);
		expect(document.activeElement?.textContent).toBe('Put back');
	});

	it('falls back to the heading when no candidate row survives', () => {
		restoreRowFocus(document, gone(), [], [heading]);
		expect(document.activeElement).toBe(heading);
	});

	it('skips a fallback that has left the document', () => {
		const removed = document.createElement('h2');
		removed.tabIndex = -1;
		restoreRowFocus(document, gone(), [], [removed, undefined, heading]);
		expect(document.activeElement).toBe(heading);
	});

	// A failed action leaves the row, and its button, where they were.
	it('does nothing when the activating control is still in the document', () => {
		const survivor = document.querySelector('button')!;
		restoreRowFocus(document, survivor, ['a'], [heading]);
		expect(document.activeElement).toBe(document.body);
	});

	// Only a DROPPED focus is repaired. A user who moved on while the request was in
	// flight keeps the place they moved to.
	it('leaves focus alone when it is already somewhere other than the body', () => {
		const edit = document.querySelectorAll('button')[0] as HTMLElement;
		edit.focus();
		restoreRowFocus(document, gone(), ['b'], [heading]);
		expect(document.activeElement).toBe(edit);
	});
});

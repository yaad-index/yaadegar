import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';
import type { PageData } from './$types';

// #347: the giver page delivered every item's image_url to the browser and drew none
// of them — the data was there and the template never rendered it. An image is most of
// what tells a stranger which product is meant, so its absence made adding a picture
// appear to work while never reaching the only audience this page has.
//
// The list below deliberately mixes both kinds of item. A list where every item has a
// picture cannot show the missing-image case, and that is the case a real list hits
// first.
//
// ⚠️ Bounds are NOT asserted here: jsdom performs no layout, so `object-cover` and the
// fixed box cannot be checked from this suite. That was verified in a real browser — a
// 160x90 source and a 120x120 source both render in a 48x48 box — and those numbers
// are in the pull request, not here.

const listData = (): PageData =>
	({
		closed: false as const,
		list: {
			title: 'Birthday',
			event_date: null,
			email_required: false,
			items: [
				{ id: 'i1', name: 'Kettle', image_url: 'https://img.example.invalid/kettle.png' },
				{ id: 'i2', name: 'Espresso machine', image_url: null },
				{ id: 'i3', name: 'Wool blanket', image_url: 'https://img.example.invalid/blanket.png' }
			]
		},
		reservedItemIds: [] as string[],
		pledged: {},
		noteHtml: {}
	}) as unknown as PageData;

const itemImages = (container: HTMLElement) => [...container.querySelectorAll('li img')];

describe('giver page item images (#347)', () => {
	it('draws the picture of every item that has one', () => {
		const { container } = render(Page, { data: listData(), form: null });
		expect(itemImages(container).map((i) => i.getAttribute('src'))).toEqual([
			'https://img.example.invalid/kettle.png',
			'https://img.example.invalid/blanket.png'
		]);
	});

	// The picture sits beside the item's visible name, so it adds nothing a screen
	// reader needs and an alt naming the item would announce it twice. These two
	// assertions belong together: alt="" is only correct WHILE the name is rendered
	// beside it, so a change that drops the visible name must fail here.
	it('marks each picture decorative, and keeps the name as visible text beside it', () => {
		const { container } = render(Page, { data: listData(), form: null });
		expect(itemImages(container).map((i) => i.getAttribute('alt'))).toEqual(['', '']);
		expect(screen.getByText('Kettle')).toBeInTheDocument();
		expect(screen.getByText('Wool blanket')).toBeInTheDocument();
	});

	it('defers loading, since this is the public page on a cold cache', () => {
		const { container } = render(Page, { data: listData(), form: null });
		expect(itemImages(container).map((i) => i.getAttribute('loading'))).toEqual(['lazy', 'lazy']);
	});

	// The half that is easy to lose: an item with no picture must still read as a row,
	// not as a gap or a broken icon. Asserting the count is what catches a fix that
	// renders an <img> for the imageless item too.
	it('leaves the generic mark on an item with no picture, and draws no image for it', () => {
		const { container } = render(Page, { data: listData(), form: null });
		expect(itemImages(container)).toHaveLength(2);
		expect(screen.getByText('Espresso machine')).toBeInTheDocument();
	});
});

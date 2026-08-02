import { error, fail } from '@sveltejs/kit';
import type { NumericRange } from '@sveltejs/kit';
import { superValidate, message } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';
import { backendClient } from '$lib/server/api';
import { capsForList, addCap, removeCap } from '$lib/server/caps';
import { contribCapsForList, addContribCap, removeContribCap } from '$lib/server/caps';
import { renderNote } from '$lib/server/markdown';
import type { Actions, PageServerLoad } from './$types';

// The giver may optionally leave a name and an email; both are used only
// server-side (e.g. decay reminders) and are never shown to anyone else. The item
// is chosen by the clicked Reserve button (name="item_id"). Quantity defaults to 1
// server-side, so the giver flow does not surface it.
const reserveSchema = z.object({
	item_id: z.string().min(1),
	giver_name: z.string().max(200).optional(),
	giver_email: z.string().email('Enter a valid email').optional().or(z.literal(''))
});

// A co-buying pledge: a share of the item's price plus a contact that is revealed
// only after a mutually-confirmed match (ADR-0002 §6). The amount is entered in
// major currency units; the currency rides a hidden field from the item's price
// and the backend rejects any mismatch. giver_name is optional.
const pledgeSchema = z.object({
	item_id: z.string().min(1),
	amount: z.number().positive('Enter an amount greater than zero'),
	currency: z.string().min(1),
	contact_email: z.string().email('Enter a valid email'),
	giver_name: z.string().max(200).optional()
});

// PledgeState is this browser's own view of a pledge it made — derived from its
// capability cookie + a server-side status read, never from the list payload.
interface PledgeState {
	contribution_id: string;
	status: string;
	matched: boolean;
	match_id: string | null;
}

const isSecure = (url: URL) => url.protocol === 'https:';

// On a failed public action, surface the backend's problem+json `detail` when it is a
// 4xx — these details are written to be shown to the giver ("an email address is
// required to reserve on this list", "that email address is not valid", "item not
// found"). Without this the reserve/pledge actions collapsed every non-409/410 failure
// into a generic retry message, hiding the real reason (e.g. the email-confirm reserver
// tier needs an email, while the form calls it optional). 5xx details are never shown —
// they can leak internals — so those, and any response with no usable detail, fall back
// to the caller's generic message with a 400. `err` is openapi-fetch's parsed body.
function backendReason(
	err: unknown,
	response: { status?: number } | undefined,
	fallback: string
): { text: string; status: NumericRange<400, 599> } {
	const status = response?.status ?? 0;
	const detail = (err as { detail?: unknown } | null | undefined)?.detail;
	if (status >= 400 && status < 500 && typeof detail === 'string' && detail.trim() !== '') {
		return { text: detail, status: status as NumericRange<400, 599> };
	}
	return { text: fallback, status: 400 };
}

export const load: PageServerLoad = async ({ params, locals, cookies, url }) => {
	const client = backendClient({ host: locals.host });
	const {
		data,
		error: err,
		response
	} = await client.GET('/public/{shareSlug}', {
		params: { path: { shareSlug: params.shareSlug } }
	});
	const form = await superValidate(zod4(reserveSchema));
	const pledgeForm = await superValidate(zod4(pledgeSchema), { id: 'pledge' });
	if (err || !data) {
		// A disabled list, or one past its event date, is a clean "closed" state — not
		// an error page (ADR-0002: the backend signals it with 410).
		if (response.status === 410) {
			return {
				closed: true as const,
				list: null,
				reservedItemIds: [] as string[],
				pledged: {} as Record<string, PledgeState>,
				noteHtml: {} as Record<string, string>,
				form,
				pledgeForm
			};
		}
		error(response.status === 404 ? 404 : response.status || 502, 'This list is not available.');
	}

	// This browser's own pledges: refresh each on load (the approved model is manual
	// check + on-load refresh, no auto-poll). A stale capability — the contribution
	// was withdrawn/declined out from under it — is dropped so it stops showing.
	const myContribs = contribCapsForList(cookies, params.shareSlug);
	const pledged: Record<string, PledgeState> = {};
	for (const [itemId, entry] of Object.entries(myContribs)) {
		const { data: c, response: cr } = await client.GET('/public/contributions/{contributionId}', {
			params: { path: { contributionId: entry.contribution_id } },
			headers: { 'X-Capability-Token': entry.token }
		});
		if (c) {
			// Terminal pledges are spent — forget them so the item reverts to its normal
			// offer state for this browser. `expired` is the auto-expiry sweep's outcome
			// (#101); without it the pledged panel would keep showing a stale "you're
			// chipping in" with a dead handshake link.
			if (c.status === 'declined' || c.status === 'withdrawn' || c.status === 'expired') {
				removeContribCap(cookies, params.shareSlug, itemId, isSecure(url));
				continue;
			}
			pledged[itemId] = {
				contribution_id: entry.contribution_id,
				status: c.status ?? 'pending',
				matched: !!c.match_id,
				match_id: c.match_id ?? null
			};
			// Record the match id on the stored cap so the same-browser handshake link
			// can be a clean /cobuy/<matchId> and the token resolved server-side.
			if (c.match_id && entry.match_id !== c.match_id) {
				addContribCap(
					cookies,
					params.shareSlug,
					itemId,
					{ contribution_id: entry.contribution_id, token: entry.token, match_id: c.match_id },
					isSecure(url)
				);
			}
		} else if (cr?.status === 401 || cr?.status === 404) {
			removeContribCap(cookies, params.shareSlug, itemId, isSecure(url));
		}
	}

	return {
		closed: false as const,
		list: data,
		// Only the IDs of items THIS browser reserved — enough to drive the "you
		// reserved this" state and the release button. The capability tokens stay in
		// the httpOnly cookie and are read server-side in the release action; they are
		// deliberately NOT returned here, so they never reach client JS (ADR-0006 §4).
		reservedItemIds: Object.keys(capsForList(cookies, params.shareSlug)),
		// This browser's own co-buy pledges (id + status), same server-only token rule.
		pledged,
		// Notes rendered to sanitized HTML server-side; {@html} only touches this map.
		noteHtml: Object.fromEntries((data.items ?? []).map((i) => [i.id ?? '', renderNote(i.note)])),
		form,
		pledgeForm
	};
};

export const actions: Actions = {
	reserve: async ({ request, params, locals, cookies, url }) => {
		const form = await superValidate(request, zod4(reserveSchema));
		if (!form.valid) return fail(400, { form });

		const client = backendClient({ host: locals.host });
		const {
			data,
			error: err,
			response
		} = await client.POST('/public/{shareSlug}/items/{itemId}/reservations', {
			params: { path: { shareSlug: params.shareSlug, itemId: form.data.item_id } },
			body: {
				giver_name: form.data.giver_name || null,
				giver_email: form.data.giver_email || null,
				// The giver flow doesn't surface quantity; reserve a single unit. The spec
				// marks quantity required though it defaults to 1 server-side.
				quantity: 1
			}
		});
		if (err || !data?.reservation_id || !data?.capability_token) {
			if (response?.status === 409)
				return message(form, 'Someone just reserved this one.', { status: 409 });
			if (response?.status === 410)
				return message(form, 'This list is no longer active.', { status: 410 });
			const reason = backendReason(err, response, 'Could not reserve this item. Please try again.');
			return message(form, reason.text, { status: reason.status });
		}
		// Persist the one-time token server-side; it never touches client JS.
		addCap(
			cookies,
			params.shareSlug,
			form.data.item_id,
			{ reservation_id: data.reservation_id, token: data.capability_token },
			isSecure(url)
		);
		return message(form, 'Reserved — thank you!');
	},

	release: async ({ request, params, locals, cookies, url }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, { releaseError: 'Nothing to release.' });

		const entry = capsForList(cookies, params.shareSlug)[itemId];
		if (!entry) return fail(400, { releaseError: 'No reservation to release.' });

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.DELETE('/public/reservations/{reservationId}', {
			params: { path: { reservationId: entry.reservation_id } },
			headers: { 'X-Capability-Token': entry.token }
		});
		// 204 = released. 401/404 means the reservation is already gone (released
		// elsewhere or auto-expired) — the stored capability is stale either way, so
		// drop it and show the item as available again.
		if (!err || response?.status === 401 || response?.status === 404) {
			removeCap(cookies, params.shareSlug, itemId, isSecure(url));
			return { released: true };
		}
		return fail(502, { releaseError: 'Could not release this reservation. Please try again.' });
	},

	pledge: async ({ request, params, locals, cookies, url }) => {
		const form = await superValidate(request, zod4(pledgeSchema), { id: 'pledge' });
		if (!form.valid) return fail(400, { pledgeForm: form });

		const client = backendClient({ host: locals.host });
		// Amount is entered in major units; the backend money model is minor units.
		const amountMinor = Math.round(form.data.amount * 100);
		const {
			data,
			error: err,
			response
		} = await client.POST('/public/{shareSlug}/items/{itemId}/contributions', {
			params: { path: { shareSlug: params.shareSlug, itemId: form.data.item_id } },
			body: {
				pledged: { amount_minor: amountMinor, currency: form.data.currency },
				contact_email: form.data.contact_email,
				giver_name: form.data.giver_name || null
			}
		});
		if (err || !data?.contribution_id || !data?.capability_token) {
			if (response?.status === 409)
				return fail(409, {
					pledgeForm: form,
					pledgeError: 'This item was just fully funded or reserved.'
				});
			if (response?.status === 410)
				return fail(410, { pledgeForm: form, pledgeError: 'This list is no longer active.' });
			const reason = backendReason(
				err,
				response,
				'Could not record your pledge. Please try again.'
			);
			return fail(reason.status, { pledgeForm: form, pledgeError: reason.text });
		}
		// Persist the pledge capability server-side (never to client JS).
		addContribCap(
			cookies,
			params.shareSlug,
			form.data.item_id,
			{ contribution_id: data.contribution_id, token: data.capability_token },
			isSecure(url)
		);
		// A match forms the moment pending pledges cover the price; the giver confirms
		// it from the link emailed to them (the /cobuy handshake).
		const pledgeMessage = data.match
			? 'Thanks — that completes the gift! Check your email to confirm the group buy.'
			: "Thanks for chipping in! We'll email you if a match forms.";
		return { pledgeMessage };
	},

	withdraw: async ({ request, params, locals, cookies, url }) => {
		const fd = await request.formData();
		const itemId = String(fd.get('item_id') ?? '');
		if (!itemId) return fail(400, { withdrawError: 'Nothing to withdraw.' });

		const entry = contribCapsForList(cookies, params.shareSlug)[itemId];
		if (!entry) return fail(400, { withdrawError: 'No pledge to withdraw.' });

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.DELETE('/public/contributions/{contributionId}', {
			params: { path: { contributionId: entry.contribution_id } },
			headers: { 'X-Capability-Token': entry.token }
		});
		// 204 = withdrawn. 401/404 = already gone → drop the stale capability. 409 =
		// the match is already both-confirmed, so withdrawal is no longer allowed; keep
		// the capability and tell the giver.
		if (!err || response?.status === 401 || response?.status === 404) {
			removeContribCap(cookies, params.shareSlug, itemId, isSecure(url));
			return { withdrawn: true };
		}
		if (response?.status === 409) {
			return fail(409, {
				withdrawError: 'This group buy is already confirmed and cannot be withdrawn.'
			});
		}
		return fail(502, { withdrawError: 'Could not withdraw your pledge. Please try again.' });
	},

	// refresh re-runs load (which re-reads each pledge's status), so the giver can
	// check whether a match has formed without an account or auto-polling.
	refresh: async () => {
		return { refreshed: true };
	}
};

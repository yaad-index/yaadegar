import { describe, it, expect, vi } from 'vitest';

// #144: the public reserve + chip-in actions must surface the backend's problem+json
// `detail` on a 4xx failure (the giver-facing reason, e.g. "an email address is
// required to reserve on this list") instead of the old generic retry message, while
// NEVER leaking a 5xx detail. backendClient is mocked so the POST returns a chosen
// {data,error,response}; `state.next` is what the next POST resolves to.
const state = vi.hoisted(() => ({ next: null as unknown }));
vi.mock('$lib/server/api', () => ({
	backendClient: () => ({
		POST: async () => state.next
	})
}));

import { actions } from './+page.server';

// The reserve/pledge actions read only these off the event; the failure paths never
// touch cookies or url, so minimal stubs suffice.
const ev = (name: string, fields: Record<string, string>) => {
	const fd = new FormData();
	for (const [k, v] of Object.entries(fields)) fd.set(k, v);
	return {
		request: new Request('http://t.example/l/s1?/' + name, { method: 'POST', body: fd }),
		params: { shareSlug: 's1' },
		locals: { host: 't.example' },
		cookies: { get: () => undefined, set: () => {}, delete: () => {} },
		url: new URL('http://t.example/l/s1')
	};
};

type ActionFn = (e: ReturnType<typeof ev>) => Promise<unknown>;
const callReserve = (fields: Record<string, string>, next: unknown) => {
	state.next = next;
	return (actions.reserve as unknown as ActionFn)(ev('reserve', fields));
};
const callPledge = (fields: Record<string, string>, next: unknown) => {
	state.next = next;
	return (actions.pledge as unknown as ActionFn)(ev('pledge', fields));
};

// message(form, txt, {status>=400}) returns fail(status, {form}); a message with no
// status returns {form}. Read whichever the failure produced.
const msgOf = (res: unknown): string | undefined => {
	const r = res as { data?: { form?: { message?: string } }; form?: { message?: string } };
	return r.data?.form?.message ?? r.form?.message;
};
const statusOf = (res: unknown): number | undefined => (res as { status?: number }).status;
const pledgeErrOf = (res: unknown): string | undefined =>
	(res as { data?: { pledgeError?: string } }).data?.pledgeError;

const okReserveFields = { item_id: 'i1', giver_email: 'g@example.com' };
const okPledgeFields = {
	item_id: 'i1',
	amount: '10',
	currency: 'USD',
	contact_email: 'g@example.com'
};

describe('reserve action surfaces the backend reason (#144)', () => {
	it('shows the 4xx problem+json detail instead of the generic message', async () => {
		const detail = 'an email address is required to reserve on this list';
		const res = await callReserve(okReserveFields, {
			data: undefined,
			error: { detail },
			response: { status: 400 }
		});
		expect(msgOf(res)).toBe(detail);
		expect(statusOf(res)).toBe(400);
	});

	it('falls back to the generic message on a 5xx (never leaks internal detail)', async () => {
		const res = await callReserve(okReserveFields, {
			data: undefined,
			error: { detail: 'boom: db connection refused at 10.0.0.5' },
			response: { status: 500 }
		});
		expect(msgOf(res)).toBe('Could not reserve this item. Please try again.');
	});

	it('keeps the 409 already-reserved special-case', async () => {
		const res = await callReserve(okReserveFields, {
			data: undefined,
			error: { detail: 'conflict' },
			response: { status: 409 }
		});
		expect(msgOf(res)).toBe('Someone just reserved this one.');
		expect(statusOf(res)).toBe(409);
	});

	it('treats a 202 pending_confirmation (no token) as success, not failure', async () => {
		// The email-confirm primary path: the reservation IS created and a confirm email
		// sent; no capability token exists yet. This must read as success, not the generic
		// error (the pre-fix guard mis-classified the missing token as a failure).
		const res = await callReserve(okReserveFields, {
			data: { reservation_id: 'r1', status: 'pending_confirmation' },
			error: undefined,
			response: { status: 202 }
		});
		expect(msgOf(res)).toBe('Almost there — check your email to confirm your reservation.');
		expect(statusOf(res)).toBeUndefined(); // a success message, not a fail()
	});

	it('keeps the 201 active-with-token path as a confirmed reserve', async () => {
		const res = await callReserve(okReserveFields, {
			data: { reservation_id: 'r1', status: 'active', capability_token: 'tok' },
			error: undefined,
			response: { status: 201 }
		});
		expect(msgOf(res)).toBe('Reserved — thank you!');
	});
});

describe('pledge action surfaces the backend reason (#144)', () => {
	it('shows the 4xx problem+json detail instead of the generic message', async () => {
		const detail = 'that email address is not valid';
		const res = await callPledge(okPledgeFields, {
			data: undefined,
			error: { detail },
			response: { status: 400 }
		});
		expect(pledgeErrOf(res)).toBe(detail);
		expect(statusOf(res)).toBe(400);
	});

	it('falls back to the generic message on a 5xx', async () => {
		const res = await callPledge(okPledgeFields, {
			data: undefined,
			error: { detail: 'panic: nil pointer in ledger' },
			response: { status: 500 }
		});
		expect(pledgeErrOf(res)).toBe('Could not record your pledge. Please try again.');
	});
});

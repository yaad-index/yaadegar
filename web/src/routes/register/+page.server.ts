import { fail } from '@sveltejs/kit';
import { backendClient } from '$lib/server/api';
import { safeReturnTo, returnToCookie } from '$lib/server/returnTo';
import type { Actions, PageServerLoad } from './$types';

// Carry a validated local return path (#170) through registration: the page echoes it
// into the form action, and the action stashes it in a short-lived cookie so the later
// email-verify step (a separate request with no query to ride) can land the new account
// back on the list it started from.
export const load: PageServerLoad = async ({ locals, url }) => {
	// Agree with the instance policy up front (#253): when self-registration is disabled,
	// the page renders a short not-enabled state instead of a form that can only 403. This
	// reads the same registration_enabled signal the login page already uses to hide its
	// "Sign up" entry point, so the entry point and the destination agree. The action's
	// 403 handling stays as the authoritative server-side guard; this only stops
	// presenting an action the instance will refuse. Defaults to disabled if the backend
	// can't be reached, matching the login loader's own default.
	const client = backendClient({ host: locals.host });
	const { data: methods } = await client.GET('/api/v1/auth/methods');
	return {
		returnTo: safeReturnTo(url.searchParams.get('return_to')) ?? '',
		registrationEnabled: methods?.registration_enabled ?? false
	};
};

export const actions: Actions = {
	// Start email self-registration (ADR-0012 cut 1a). The backend is enumeration-safe:
	// on any 2xx we show the same neutral "check your email" message whether or not the
	// email already exists, so the UI never reveals which. A 403 (registration disabled
	// on this instance) shows a distinct message; other failures surface the backend
	// reason. The captcha is a no-op for now, so we send an empty captcha_token.
	default: async ({ request, locals, cookies, url }) => {
		const fd = await request.formData();
		const email = String(fd.get('email') ?? '').trim();
		const password = String(fd.get('password') ?? '');
		const confirmPassword = String(fd.get('confirm_password') ?? '');
		if (!email) return fail(400, { error: 'Enter your email.' });
		if (!password) return fail(400, { error: 'Choose a password.' });
		if (password !== confirmPassword) {
			return fail(400, { error: 'The passwords do not match.' });
		}

		const client = backendClient({ host: locals.host });
		const { error: err, response } = await client.POST('/api/v1/auth/register', {
			body: { email, password, captcha_token: '' }
		});
		if (response.status === 403) {
			return fail(403, { disabled: true });
		}
		if (err) {
			// Surface the backend's real reason (e.g. a too-short password), falling back
			// to a generic line.
			return fail(response.status, {
				error: err.detail ?? 'Something went wrong. Please try again.'
			});
		}

		// Stash a return path (#170) for the post-verify redirect. Set only on a 2xx and
		// only for a valid local path, so it never survives as an open redirect. One hour
		// mirrors the verification link's own lifetime.
		const returnTo = safeReturnTo(url.searchParams.get('return_to'));
		if (returnTo) {
			returnToCookie.set(cookies, returnTo, { maxAge: 3600, secure: url.protocol === 'https:' });
		}

		// Enumeration-neutral: the same confirmation whether the email was new or already
		// existed. No auto-login — the account is pending until it verifies its email.
		return { sent: true };
	}
};

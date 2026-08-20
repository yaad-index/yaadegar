import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import DomainDnsRecords from './DomainDnsRecords.svelte';

// #239: when no CNAME target is configured the panel must NOT render a blank value
// (indistinguishable from a value the user failed to read) — it says so explicitly.
describe('DomainDnsRecords', () => {
	const base = { hostname: 'gifts.example.test', verificationToken: 'verify-token-abc' };

	it('renders the CNAME target when configured', () => {
		render(DomainDnsRecords, { ...base, cnameTarget: 'edge.wishes.example' });
		expect(screen.getByText('edge.wishes.example')).toBeInTheDocument();
		expect(screen.queryByTestId('cname-not-configured')).toBeNull();
	});

	it('shows an explicit not-configured note instead of a blank CNAME value', () => {
		const { container } = render(DomainDnsRecords, { ...base, cnameTarget: '' });
		const note = screen.getByTestId('cname-not-configured');
		expect(note).toHaveTextContent(/no CNAME target configured/i);
		// No <dd> renders as an empty value the user would try to publish.
		for (const dd of container.querySelectorAll('dd')) {
			expect(dd.textContent?.trim()).not.toBe('');
		}
	});

	it('treats a null target the same as an empty one', () => {
		render(DomainDnsRecords, { ...base, cnameTarget: null });
		expect(screen.getByTestId('cname-not-configured')).toBeInTheDocument();
	});
});

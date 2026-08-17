import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import EmptyState from './EmptyState.svelte';

describe('EmptyState', () => {
	it('renders the title as a heading and the description', () => {
		render(EmptyState, { title: 'No lists yet', description: 'Create your first list to begin.' });
		expect(screen.getByRole('heading', { name: 'No lists yet' })).toBeInTheDocument();
		expect(screen.getByText('Create your first list to begin.')).toBeInTheDocument();
	});

	it('omits the description when none is given', () => {
		render(EmptyState, { title: 'Nothing reserved' });
		expect(screen.getByRole('heading', { name: 'Nothing reserved' })).toBeInTheDocument();
	});
});

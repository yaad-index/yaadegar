// Focus repair for list rows that an action removes (#428). When a row's own button
// deletes or moves the row, the button goes with it and the browser drops focus to
// <body>, sending a keyboard user back to the top of the page.

const CONTROL = 'a[href], button:not([disabled]), input:not([type="hidden"]), select, textarea';

// neighbourIds names the rows focus should try once `id` is gone, in order: the row
// after it (which takes its place), then the row before it.
export function neighbourIds(ids: string[], id: string): string[] {
	const i = ids.indexOf(id);
	if (i < 0) return [];
	return [ids[i + 1], ids[i - 1]].filter((n): n is string => n !== undefined);
}

// restoreRowFocus moves focus to the first control of the first surviving row in
// `candidates` (rows carry data-row-id), else to the first fallback still in the
// document. It only acts when focus was actually dropped: if the activating control
// survived (a failed action leaves the row in place) or the user has already moved
// on, focus is theirs and is left alone.
export function restoreRowFocus(
	root: ParentNode,
	activator: Element | null,
	candidates: string[],
	fallbacks: (HTMLElement | null | undefined)[]
): void {
	if (activator?.isConnected) return;
	const active = document.activeElement;
	if (active && active !== document.body) return;

	const rows = Array.from(root.querySelectorAll<HTMLElement>('[data-row-id]'));
	for (const id of candidates) {
		const row = rows.find((r) => r.dataset.rowId === id);
		const control = row?.querySelector<HTMLElement>(CONTROL);
		if (control) {
			control.focus();
			return;
		}
	}
	fallbacks.find((el) => el?.isConnected)?.focus();
}

import { superValidate } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { z } from 'zod';

// A trivial schema/form proving the forms + validation stack is wired (ADR-0006
// §4). Superforms 2.x ships zod 4, so we use its zod4 adapter. No feature behavior —
// F1 is foundations only.
const demoSchema = z.object({ name: z.string().default('') });

export const load = async () => {
	return { form: await superValidate(zod4(demoSchema)) };
};

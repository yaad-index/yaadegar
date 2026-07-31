import { describe, it, expect } from 'vitest';
import { txtRecordName, TXT_VERIFY_PREFIX } from './domains';

describe('txtRecordName', () => {
	it('prefixes the hostname with the verification label', () => {
		expect(txtRecordName('gifts.example.com')).toBe('_yaadegar-verify.gifts.example.com');
	});

	it('uses the shared prefix constant', () => {
		expect(txtRecordName('a.example.test')).toBe(`${TXT_VERIFY_PREFIX}.a.example.test`);
	});
});

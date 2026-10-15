import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ConfirmDialog } from './ConfirmDialog';

describe('ConfirmDialog', () => {
    it('hanya konfirmasi bila nama target diketik persis', async () => {
        const user = userEvent.setup();
        const onConfirm = vi.fn();
        const onOpenChange = vi.fn();
        render(
            <ConfirmDialog target="bca" open onOpenChange={onOpenChange} onConfirm={onConfirm} />,
        );
        const input = screen.getByRole('textbox');
        await user.type(input, 'salah');
        await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
        expect(onConfirm).not.toHaveBeenCalled();
        await user.clear(input);
        await user.type(input, 'bca');
        await user.click(screen.getByRole('button', { name: 'Konfirmasi' }));
        expect(onConfirm).toHaveBeenCalledOnce();
    });
});

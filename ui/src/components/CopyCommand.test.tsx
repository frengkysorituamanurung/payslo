import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { CopyCommand } from './CopyCommand';

describe('CopyCommand', () => {
    it('menyalin perintah ke clipboard', async () => {
        const user = userEvent.setup();
        const writeText = vi.fn().mockResolvedValue(undefined);
        Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
        render(<CopyCommand command="make up-lite" />);
        await user.click(screen.getByRole('button', { name: 'Salin perintah' }));
        expect(writeText).toHaveBeenCalledWith('make up-lite');
        expect(await screen.findByRole('button', { name: 'Disalin' })).toBeInTheDocument();
    });
});

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DiffViewer } from './DiffViewer';

describe('DiffViewer', () => {
    it('menampilkan baris tambah/hapus dengan ringkasan', () => {
        render(<DiffViewer actual={'a\nb\nc\n'} desired={'a\nB\nc\n'} note="" />);
        expect(screen.getByText(/1 baris tambah, 1 baris hapus/)).toBeInTheDocument();
        expect(screen.getByRole('cell', { name: 'B' })).toBeInTheDocument();
    });

    it('jujur saat tak ada teks: tampilkan catatan, bukan diff kosong', () => {
        render(<DiffViewer actual="" desired="" note="catatan backend" />);
        expect(screen.getByText('catatan backend')).toBeInTheDocument();
    });
});

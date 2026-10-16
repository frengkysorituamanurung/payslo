package main

// Renderer Markdown -> HTML U6 (halaman Belajar). Prinsip keamanan (uji di
// u6_test.go TestMarkdownSanitized):
//   - TIDAK ADA raw HTML: semua '<' di-escape menjadi &lt;. <script>, <iframe>,
//     <img onerror> TIDAK PERNAH muncul sebagai markup.
//   - HANYA tag yang dihasilkan renderer ini yang boleh ada di output.
//   - Link <a> hanya untuk skema http/https/mailto; javascript:, data:, dan
//     skema lain dirender sebagai TEKS polos (bukan link).
//
// Ini penyelarasan pertama; UI tetap menjalankan DOMPurify sebelum
// dangerouslySetInnerHTML (lapisan kedua, per UI-SPEC).

import (
	"strings"
)

// renderMarkdown mengubah markdown menjadi HTML aman. Cukup untuk kurikulum:
// heading, paragraf, daftar (terurut/terurut), kode inline/blok, tebal,
// miring, link. Tabel dan gambar sengaja tidak didukung (kurikulum tak
// memakainya; permintaan fitur dicatat di backlog UI).
func renderMarkdown(src string) string {
	lines := strings.Split(src, "\n")
	var out strings.Builder
	out.Grow(len(src) + len(src)/2 + 16)

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Baris kosong: akhiri blok berjalan.
		if trimmed == "" {
			i++
			continue
		}

		// Blok kode ``` (sampai penutup).
		if strings.HasPrefix(trimmed, "```") {
			i++
			var code strings.Builder
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code.WriteString(lines[i])
				code.WriteByte('\n')
				i++
			}
			if i < len(lines) {
				i++ // lewati penutup ```
			}
			out.WriteString("<pre><code>")
			out.WriteString(escapeHTML(code.String()))
			out.WriteString("</code></pre>\n")
			continue
		}

		// Heading #..####
		if h, level, ok := headingLine(trimmed); ok {
			out.WriteString("<h" + string(rune('0'+level)) + ">")
			out.WriteString(renderInline(h))
			out.WriteString("</h" + string(rune('0'+level)) + ">\n")
			i++
			continue
		}

		// Daftar tak terurut (- atau *).
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			var items strings.Builder
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if t == "" || (!strings.HasPrefix(t, "- ") && !strings.HasPrefix(t, "* ")) {
					break
				}
				items.WriteString("<li>")
				items.WriteString(renderInline(t[2:]))
				items.WriteString("</li>\n")
				i++
			}
			out.WriteString("<ul>\n")
			out.WriteString(items.String())
			out.WriteString("</ul>\n")
			continue
		}

		// Daftar terurut (1. dst).
		if orderedItem(trimmed) != "" {
			var items strings.Builder
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				rest := orderedItem(t)
				if rest == "" {
					break
				}
				items.WriteString("<li>")
				items.WriteString(renderInline(rest))
				items.WriteString("</li>\n")
				i++
			}
			out.WriteString("<ol>\n")
			out.WriteString(items.String())
			out.WriteString("</ol>\n")
			continue
		}

		// Paragraf: kumpulkan baris sampai kosong/daftar/heading.
		var para strings.Builder
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if t == "" {
				break
			}
			if _, _, ok := headingLine(t); ok {
				break
			}
			if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || orderedItem(t) != "" || strings.HasPrefix(t, "```") {
				break
			}
			if para.Len() > 0 {
				para.WriteByte('\n')
			}
			para.WriteString(t)
			i++
		}
		out.WriteString("<p>")
		out.WriteString(renderInline(para.String()))
		out.WriteString("</p>\n")
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func headingLine(s string) (string, int, bool) {
	level := 0
	for level < len(s) && s[level] == '#' {
		level++
	}
	if level == 0 || level > 4 {
		return "", 0, false
	}
	if level >= len(s) || s[level] != ' ' {
		return "", 0, false
	}
	return strings.TrimSpace(s[level:]), level, true
}

// orderedItem mengembalikan isi item daftar terurut ("1. foo" -> "foo"),
// atau "" bila bukan daftar terurut.
func orderedItem(s string) string {
	dot := strings.IndexByte(s, '.')
	if dot <= 0 {
		return ""
	}
	for _, c := range s[:dot] {
		if c < '0' || c > '9' {
			return ""
		}
	}
	rest := s[dot+1:]
	if !strings.HasPrefix(rest, " ") {
		return ""
	}
	return strings.TrimSpace(rest)
}

// escapeHTML men-escape <, >, &, ". Dipakai untuk SEMUA teks dan HTML mentah:
// tidak ada input yang lolos sebagai markup.
func escapeHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// renderInline menangani **tebal**, *miring*, `kode`, dan [link](url).
// HTML mentah di-escape; link hanya http/https/mailto.
func renderInline(s string) string {
	// Lewati dulu escape mentah, lalu terapkan inline markup pada hasilnya.
	// Pendekatan dua tahap: token polos (kode) ditandai agar tak tertimpa.
	var tokens [][2]string // {token, bentuk asli yang sudah di-escape}
	work := s

	// Kode inline `...`: ambil isinya sebelum proses lain.
	for {
		start := strings.Index(work, "`")
		if start < 0 {
			break
		}
		end := strings.Index(work[start+1:], "`")
		if end < 0 {
			break
		}
		inner := work[start+1 : start+1+end]
		token := "\x00" + itoa(len(tokens)) + "\x00"
		tokens = append(tokens, [2]string{token, "<code>" + escapeHTML(inner) + "</code>"})
		work = work[:start] + token + work[start+1+end+1:]
	}

	work = renderBoldItalic(escapeHTML(work))
	work = renderLinks(work)

	// Kembalikan token kode (isi sudah di-escape; tag <code> milik renderer).
	for _, tok := range tokens {
		work = strings.Replace(work, tok[0], tok[1], 1)
	}
	return work
}

func renderBoldItalic(s string) string {
	// **tebal**
	for {
		start := strings.Index(s, "**")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+2:], "**")
		if end < 0 {
			break
		}
		inner := s[start+2 : start+2+end]
		s = s[:start] + "<strong>" + inner + "</strong>" + s[start+2+end+2:]
	}
	// *miring*
	for {
		start := strings.Index(s, "*")
		if start < 0 {
			break
		}
		end := strings.Index(s[start+1:], "*")
		if end < 0 {
			break
		}
		inner := s[start+1 : start+1+end]
		s = s[:start] + "<em>" + inner + "</em>" + s[start+1+end+1:]
	}
	return s
}

// renderLinks: [teks](url) -> <a href="url">teks</a> hanya untuk skema
// diperbolehkan; selain itu teks polos (tanpa <a>). URL di-escape; skema
// dicek SETELAH normalisasi (lowercase, buang kontrol/spasi di awal).
func renderLinks(s string) string {
	for {
		start := strings.Index(s, "[")
		if start < 0 {
			return s
		}
		mid := strings.Index(s[start:], "](")
		if mid < 0 {
			return s
		}
		mid += start
		end := strings.Index(s[mid+2:], ")")
		if end < 0 {
			return s
		}
		end += mid + 2
		text := s[start+1 : mid]
		rawURL := strings.TrimSpace(s[mid+2 : end])
		if safeURL(rawURL) {
			s = s[:start] + "<a href=\"" + escapeHTML(rawURL) + "\">" + text + "</a>" + s[end+1:]
			continue
		}
		// Skema berbahaya: buang link, teks tetap tampil.
		s = s[:start] + text + s[end+1:]
	}
}

func safeURL(raw string) bool {
	// Buang whitespace/kontrol di awal (browser mengabaikan).
	u := strings.TrimLeft(raw, " \t\n\r\v\f\x00")
	u = strings.ToLower(u)
	for _, p := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(u, p) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

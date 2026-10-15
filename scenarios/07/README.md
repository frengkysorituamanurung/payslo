# Skenario 07 — Alert noise: flapping burn-rate cepat

> Briefing trainee. Cara mendeteksi dan memitigasi TIDAK dibocorkan di sini.
> Butuh bantuan? `lab hint 07` (maks 3, penalti kecil).

## Situasi

Pager berdering berulang kali untuk SLO yang sama: alert `page` menyala lalu
padam sendiri setiap beberapa menit. Burn-rate lambat (`ticket`) menyala
terus. Apakah ini darurat atau noise?

## Tugas

1. Bandingkan fast-burn vs slow-burn di Prometheus (`ALERTS` + recording
   `payslo:slo_error_ratio:*`) dan tentukan: transien atau sustained?
2. Atasi akar masalahnya (bukan alertnya — jangan edit threshold di lab ini;
   tuning hanya diusulkan lewat jawaban).
3. Jawab (`lab answer <id> "<teks>"`):
   - `diagnosis`: burn mana yang menyala + artinya (sebut kata "window").
   - `tuning`: usulan tuning threshold/window + contoh angka (sebut kata
     "threshold" dan sebuah angka, mis. "20").
4. `lab verify 07` harus PASS, lalu `lab scenario stop 07`.

## Aturan main

- Dilarang menambah chaos baru atau deploy versi `bad` (skor safety 0).
- Jangan ubah file SLO/rules (cukup usulkan tuning di jawaban).

% rc4_prolog.pl — RC4 stream cipher in (real) Prolog.
%
% A faithful port of the Perl rc4.pl: same key "Key", same plaintext
% "Plaintext", same expected ciphertext vector, and the same
% "yay!"/"boo." verdict. (Yes, it's actually Prolog this time.)
%
% Depends on SWI-Prolog. Run the self-check with:
%   swipl -g "rc4:rc4_test" -t halt rc4_prolog.pl

:- module(rc4, [rc4_new/3, rc4_generate/3, rc4_encrypt/4, rc4_test/0]).
:- use_module(library(lists)).

% State is a term state(SBox, I, J).

% --- KSA: key scheduling algorithm ---
rc4_new(Key, state(SBox, 0, 0), SBox) :-
    string_codes(Key, KeyCodes),
    numlist(0, 255, Identity),
    ksa(KeyCodes, 0, 0, Identity, SBox).

% ksa(Key, Idx, J, SIn, SOut)
ksa(_, Idx, _, S, S) :- Idx >= 256, !.
ksa(Key, Idx, J, SIn, SOut) :-
    length(Key, KeyLen),
    KeyIdx is Idx mod KeyLen,
    nth0(KeyIdx, Key, K),
    nth0(Idx, SIn, SI),
    J2 is (J + SI + K) mod 256,
    swap_at(SIn, Idx, J2, S2),
    Idx2 is Idx + 1,
    ksa(Key, Idx2, J2, S2, SOut).

% --- PRGA: generate one keystream byte ---
rc4_generate(state(SIn, I, J), state(SOut, I2, J2), Byte) :-
    I2 is (I + 1) mod 256,
    nth0(I2, SIn, SI),
    J2 is (J + SI) mod 256,
    swap_at(SIn, I2, J2, S2),
    nth0(I2, S2, A),
    nth0(J2, S2, B),
    OutIdx is (A + B) mod 256,
    nth0(OutIdx, S2, Byte),
    SOut = S2.

% --- encrypt: XOR plaintext bytes with keystream ---
rc4_encrypt(St0, Plain, Cipher, StN) :-
    (   string(Plain)
    ->  string_codes(Plain, Codes)
    ;   Codes = Plain
    ),
    enc_loop(St0, Codes, [], RevCipher, StN),
    reverse(RevCipher, Cipher).

enc_loop(St, [], Acc, Acc, St).
enc_loop(St0, [P|Ps], Acc, Out, StN) :-
    rc4_generate(St0, St1, G),
    C is P xor G,
    enc_loop(St1, Ps, [C|Acc], Out, StN).

% --- helpers ---
swap_at(L, I, J, L2) :-
    nth0(I, L, VI),
    nth0(J, L, VJ),
    replace_at(L, I, VJ, L1),
    replace_at(L1, J, VI, L2).

replace_at([_|T], 0, V, [V|T]).
replace_at([H|T], I, V, [H|T2]) :-
    I > 0,
    I2 is I - 1,
    replace_at(T, I2, V, T2).

dump_sbox(SBox) :-
    write('S[0..9]: '),
    dump_hex(SBox, 0, 10),
    nl.

dump_hex(_, Idx, Max) :- Idx >= Max.
dump_hex(SBox, Idx, Max) :-
    Idx < Max,
    nth0(Idx, SBox, V),
    format('~16r ', [V]),
    Idx2 is Idx + 1,
    dump_hex(SBox, Idx2, Max).

% --- self-check (mirrors the Perl version) ---
rc4_test :-
    rc4_new("Key", St0, SBox),
    dump_sbox(SBox),
    rc4_encrypt(St0, "Plaintext", Cipher, _),
    Expected = [0xBB, 0xF3, 0x16, 0xE8, 0xD9, 0x40, 0xAF, 0x0A, 0xD3],
    (   Cipher = Expected
    ->  write('yay!'), nl
    ;   write('boo.'), nl,
        write('expected: '), dump_hex(Expected, 0, 9), nl,
        write('got:      '), dump_hex(Cipher, 0, 9), nl
    ).

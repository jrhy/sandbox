#!/usr/bin/env perl
use strict;
use warnings;

my @s;
my $i = 0;
my $j = 0;
my @key = unpack("C*", "Key");

sub ksa {
    for my $idx (0 .. 255) {
        $s[$idx] = $idx;
    }
    
    for my $idx (0 .. 255) {
        $j = ($j + $s[$idx] + $key[$idx % scalar(@key)]) % 256;
        @s[$idx, $j] = @s[$j, $idx];
    }
    
    print "S[0..9]: ";
    for my $idx (0 .. 9) {
        printf "%x ", $s[$idx];
    }
    print "\n";
}

sub generate {
    $i = ($i + 1) % 256;
    $j = ($j + $s[$i]) % 256;
    @s[$i, $j] = @s[$j, $i];
    return $s[($s[$i] + $s[$j]) % 256];
}

sub encrypt {
    my ($plaintext) = @_;
    my @bytes = unpack("C*", $plaintext);
    my @res;
    
    for my $idx (0 .. $#bytes) {
        my $g = generate();
        my $c = $g ^ $bytes[$idx];
        printf "p=%d generate: 0x%x, enc=0x%x\n", $bytes[$idx], $g, $c;
        push @res, $c;
    }
    
    return @res;
}

ksa();

# Reset i and j to 0 before starting PRGA (required by RC4 spec)
$i = 0;
$j = 0;

my @res = encrypt("Plaintext");
my @expected = (0xBB, 0xF3, 0x16, 0xE8, 0xD9, 0x40, 0xAF, 0x0A, 0xD3);

my $expected_str = sprintf "%02x " x scalar(@expected), @expected;
my $res_str = sprintf "%02x " x scalar(@res), @res;

print "expected: $expected_str\n";
print "got:      $res_str\n";

if ($res_str eq $expected_str) {
    print "yay!\n";
} else {
    print "boo.\n";
    for my $idx (0 .. $#res) {
        printf "%x ", $res[$idx];
    }
    print "\n";
    for my $idx (0 .. $#expected) {
        printf "%x ", $expected[$idx];
    }
    print "\n";
}


$i = 0;
$j = 0;
my @s;

@key = unpack( "C*", "Key" );

sub ksa {
	for (my $i = 0; $i < 256; $i++) {
		@s[$i] = $i;
	};
	for (my $i = 0; $i < 256; $i++) {
		$j = ($j + @s[$i] + @key[$i % (scalar @key)]) % 256;
		(@s[$i], @s[$j]) = (@s[$j], @s[$i]);
	};
	$j = 0;
	printf("S[0..9]: ");
	for (my $i = 0; $i < 10; $i++) {
		printf("%x ", @s[$i]);
	};
	printf("\n");
};

sub generate {
	$i = ($i + 1) % 256;
	$j = ($j + $s[$i]) % 256;
	(@s[$i], @s[$j]) = (@s[$j], @s[$i]);
	return @s[(@s[$i] + @s[$j]) % 256];
};

sub encrypt {
	my @bytes = unpack("C*", shift);
	#print("@bytes ");
	#printf("len %d\n", scalar @bytes);
	my @res = ();
	my $g, $c;
	for (my $i = 0; $i < scalar @bytes; $i++) {
		$g = generate();
		$c = $g ^ @bytes[$i];
		printf("p=%d generate: 0x%x, enc=0x%x\n", @bytes[$i], $g, $c);
		push @res, $c;
		#@res[$i] = $g ^ @bytes[$i];
	};
	return @res;
}

ksa();
my @res = encrypt("Plaintext");
my @expected = (0xBB, 0xF3, 0x16, 0xE8, 0xD9, 0x40, 0xAF, 0x0A, 0xD3);
if (equal((\@expected, \@res))) {
  print("yay!\n");
} else {
  print("boo.\n");
	for (my $i = 0; $i < scalar @res; $i++) {
		printf("%x ", @res[$i]);
	};
	printf("\n");
	for (my $i = 0; $i < scalar @expected; $i++) {
		printf("%x ", @expected[$i]);
	};
	printf("\n");
};

sub equal {
  my (@left, @right) = (@{$_[0]}, @{$_[1]});
printf("left:\n");
	for (my $i = 0; $i < scalar @left; $i++) {
		printf("%x ", @left[$i]);
	};
	printf("\n");
printf("right:\n");
	for (my $i = 0; $i < scalar @right; $i++) {
		printf("%x ", @right[$i]);
	};
	printf("\n");
  if (scalar @left != scalar @right) { return 0; }
  for (my $i = 0; $i < scalar @left; $i++) {
    printf("comparing %x %x\n", @left[$i], @right[$i]);
    if (@left[$i] != @right[$i]) { return 0; };
  }
  return 1;
}

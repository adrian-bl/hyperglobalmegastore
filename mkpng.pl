#!/usr/bin/perl
use strict;
use String::CRC32 qw(crc32);
use Compress::Zlib;
use POSIX qw(ceil);

use constant ENC_AES => 'aes128';

my $tmpout = "${$}tmp.bin";
my $encout = $tmpout.ENC_AES;

foreach my $to_upload (@ARGV) {
	print "Converting '$to_upload' ...\n";
	
	print " ...encrypting to $encout\n";
	
	my $IV = getIV();
	
	system("./hgmcmd", "encrypt", "wurstsalat", $IV, $to_upload, $encout);
	
	my $original_csize = (-s $to_upload);
	
	unless (open(FH, "<", $encout)) {
		warn "could not open $encout: $!\n";
		next;
	}
	unlink($encout);
	
	open(OH, ">", $tmpout) or die "Could not create tempfile $tmpout :$!\n";
	convertBlob(*FH, *OH, CONTENTSIZE=>$original_csize, BLOBSIZE=>$original_csize, ENCRYPTION=>ENC_AES, LASTMODIFIED=>time(), IV=>$IV);
	close(OH);
	close(FH);
	system("flickr_upload", "--title", $to_upload, $tmpout);
}
#unlink($tmpout);


sub getIV {
	open(UR, "<", "/dev/urandom") or die "Could not open random device: $!\n";
	my $IV;
	sysread(UR, $IV, 16);
	close(UR);
	
	die "Short IV!\n" if length($IV) != 16;
	
	return $IV;
}

############################################
# Convert data at FH $ifh into PNG stored in $ofh
sub convertBlob {
	my($ifh, $ofh, %args) = @_;
	
	my $BPP   = 3; # BytesPerPixel
	my $oDF   = deflateInit();
	my $fsize = (-s FH); # not the same as CONTENTSIZE or BLOBSIZE as the encrypted INPUT is already padded
	my $sllen = ceil(sqrt($fsize/$BPP));
	my $ihdr  = pack("N N C C C C C",$sllen, $sllen, 8, 2, 0, 0, 0); # w, h, bitdepth, colortype, compression, filter, interlace
	
	# Write output data:
	print $ofh pack("H*", "89504e470d0a1a0a"); # png header magic
	print $ofh writeChunk("IHDR", $ihdr);
	
	# store metadata
	while(my($k,$v) = each(%args)) {
		print "   + meta: $k = $v\n";
		print $ofh writeChunk("tEXt", "$k=$v");
	}
	
	# Write all scanlines
	my $full_buff = '';
	my $tmp_buff  = '';
	for(my $i=0; $i<$sllen;$i++) {
		my $want_bytes = $sllen*$BPP; # with == height -> each pixel needs BPP bytes
		$want_bytes -= sysread($ifh, $tmp_buff, $want_bytes);
		
		# construct a full scanline: <FILTER><BLOB>[padding]
		$full_buff .= chr(0).$tmp_buff;
		if($want_bytes != 0) {
			$full_buff .= (chr(0) x $want_bytes);
		}
	}
	
	# deflate data
	my $dx = $oDF->deflate($full_buff);
	$dx .= $oDF->flush;
	
	print $ofh writeChunk("IDAT", $dx);
	print $ofh writeChunk("IEND", "");
	
}


############################################
# Creates a length-prefixed PNG chunk
# with an appended crc32 checksum
sub writeChunk {
	my($type, $payload) = @_;
	my $b = $type.$payload;
	$b .= pack("N", crc32($b));
	return pack("N",length($payload)).$b;
}

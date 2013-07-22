#!/usr/bin/perl
use strict;
use Flickr::Upload;
use JSON qw(to_json from_json);
use String::CRC32 qw(crc32);
use Compress::Zlib;
use POSIX qw(ceil);
use Getopt::Long;

$| = 1;

my $getopts = {};

GetOptions($getopts, "prefix|p=s") or exit 1;

my $keysize      = 256;
my $max_blobsize = 1024*1024*51;
my $metadir = "./_aliases/$getopts->{prefix}";

unless (-d $metadir) {
	system("mkdir", "-p", $metadir) and die "mkdir -p $metadir failed: $!\n";
}

foreach my $source_file (@ARGV) {
	
	unless (-f $source_file) {
		print "skipping '$source_file'\n";
		next;
	}
	
	my @source_parts = ();
	my @remote_parts = ();
	my $iv     = getRandomBytes(16); # aes block size constant
	my $key    = getRandomBytes($keysize/8);
	my $fsize  = (-s $source_file);
	
	my $encout = "tmp.encrypted.$$";
	my $pngout = "tmp.png.$$";
	my $metaout = ( split("/",$source_file) )[-1];
	   $metaout =~ tr/a-zA-Z0-9\.-/_/c;
	   $metaout = "$metadir/$metaout";
	my $json = getJSON($metaout);
	
	print "file=$source_file, meta=$metaout ($fsize bytes)...\n   ";
	
	if(exists($json->{Key})) {
		# inherit existing key. fixme: should check keylength and abort if keysize is wrong
		$key = pack("H*",$json->{Key});
		print "# metadata exists, adding new copy with same encryption key\n";
	} else {
		# no existing info: create a prototype
		$json = { Blobsize=>$max_blobsize, Created=>time(), Location=> [], Key=>unpack("H*",$key) };
		print "# creating new metadata at $metaout\n";
	}
	
	
	if($fsize > $max_blobsize) {
		system("rm x?? 2>/dev/null");
		system("split", "-b", $max_blobsize, $source_file);
		@source_parts = (sort({$a<=>$b} glob("x??")));
		print "# file split into parts: @source_parts\n";
	}
	else {
		@source_parts = ('xaa');
		symlink($source_file, 'xaa') or die "symlink() failed: $!\n";
	}
	
	foreach my $this_part (@source_parts) {
		my $blobsize = (-s $this_part);
		
		# first step is to encrypt the file using hgcmd:
		print "[$this_part] encrypting";
		system("./hgmcmd", "encrypt", unpack("H*", $key), unpack("H*", $iv), $this_part, $encout);
		unless(open(ENC_FH, "<", $encout)) {
			warn "could not open $encout: $!, skipping $this_part\n";
			next;
		}
		unlink($this_part);
		unlink($encout); # still got the open FH
		
		# ENC_FH points to the encrypted file: We can now convert it into a PNG file
		print " convert";
		open(PNG_FH, ">", $pngout) or die "Could not create tempfile $pngout: $!\n";
		convertBlob(*ENC_FH, *PNG_FH, CONTENTSIZE=>$fsize, BLOBSIZE=>$blobsize, IV=>$iv);
		close(PNG_FH);
		close(ENC_FH);
		
		# The png file is now ready at $pngout: time to upload it to flickr
		print " upload";
		my $photo_html = flickrUpload($pngout);
		print " verify";
		my $orig_photo = getFullFlickrUrl($photo_html);
		push(@remote_parts, $orig_photo);
		print "\n";
		unlink($pngout);
	}
	
	push(@{$json->{Location}}, \@remote_parts);
	storeJSON($metaout, $json);
	
}

sub getJSON {
	my($path) = @_;
	open(META, "<", $path) or return {};
	my $jref = from_json(join("", <META>));
	close(META);
	return (ref($jref) eq 'HASH' ? $jref : {});
}

sub storeJSON {
	my($metaout, $jref) = @_;
	open(META, ">", $metaout) or die "Could not write to $metaout: $!\n";
	print META to_json($jref, {utf8=>1, pretty=>1});
	close(META);
}



sub flickrUpload {
	my($to_upload) = @_;
	my $cf = flickr_parseconf();
	my $ua = Flickr::Upload->new( $cf );
	
	$ua->agent( "flickr_upload/1.0" );
	$ua->env_proxy();
	
	my $photoid = $ua->upload(photo=>$to_upload, auth_token=>$cf->{auth_token});
	my $photohtml = 'http://www.flickr.com/photos/98707671@N05/'.int($photoid)."/sizes/o/in/photostream/";
	return $photohtml;
}

################################################################
# Attempts to grab the 'original photo' img-src from given url
# the url is blindly trusted and assumed to be shell-safe
sub getFullFlickrUrl {
	my($fhtml) = @_;
	
	for(0..20) {
		my $wget_hack = `wget -q -O - $fhtml`;
		if($wget_hack =~ /<img src="([^"]+_o\.png)">/gm) {
			return $1;
		}
		sleep(5);
	}
	return undef;
}

sub flickr_parseconf {
	my $hr = { key=>'8dcf37880da64acfe8e30bb1091376b7', secret=>'2f3695d0562cdac7' };
	open(CONFIG, "<", "$ENV{HOME}/.flickrrc" ) or die "could not open $ENV{HOME}/.flickrrc\n";
	while( <CONFIG> ) {
		chomp;
		s/#.*$//;	# strip comments
		next unless m/^\s*([a-z_]+)=(.+)\s*$/io;
		$hr->{$1} = $2;
	}
	close CONFIG;
	return $hr;
}


########################################################################################
# Returns some random bytes
sub getRandomBytes {
	my($amount) = @_;
	open(UR, "<", "/dev/urandom") or die "Could not open random device: $!\n";
	my $buff;
	sysread(UR, $buff, $amount);
	close(UR);
	
	die "Short read!\n" if length($buff) != $amount;
	
	return $buff;
}

########################################################################################
# Convert data at FH $ifh into PNG stored in $ofh
sub convertBlob {
	my($ifh, $ofh, %args) = @_;
	
	my $BPP   = 3; # BytesPerPixel
	my $oDF   = deflateInit();
	my $fsize = (-s $ifh); # not the same as CONTENTSIZE or BLOBSIZE as the encrypted INPUT is already padded
	my $sllen = ceil(sqrt($fsize/$BPP));
	my $ihdr  = pack("N N C C C C C",$sllen, $sllen, 8, 2, 0, 0, 0); # w, h, bitdepth, colortype, compression, filter, interlace
	
	# Write output data:
	print $ofh pack("H*", "89504e470d0a1a0a"); # png header magic
	print $ofh writeChunk("IHDR", $ihdr);
	
	# store metadata
	while(my($k,$v) = each(%args)) {
#		print "   + meta: $k = $v\n";
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

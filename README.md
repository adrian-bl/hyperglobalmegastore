HyperGlobalMegaStore - A not-sucking cloud storage
==================================================

What is HyperGlobalMegaStore?
----------------------------------------------

HyperGlobalMegaStore encrypts and converts arbitrary data into a PNG file.
You can look at the PNG file, print it on paper or upload it to Flickr as they 
offer you 1TB of storage for each account.

The package also includes a proxy server that will decrypt & re-convert the PNG 
file on the fly from Flickr or any other image hosting provider.


But this violates the Flickr Terms of use!!!11
----------------------------------------------

Uh, maybe. Maybe not. The uploaded image files are valid PNG files and i like 
looking at them.


How to install
----------------------------------------------

Golang >= 1.3 and some Perl5 modules are required.

To compile the proxy, run:

```bash
make deps
make
```

This should produce the hgmcmd binary.

You can then launch the proxy via

```bash
./hgmcmd proxy 127.0.0.1 8080
```

Hint: You can instruct the hyperglobalmegastore-proxy to use a local forwarding proxy (such as Squid or ATS/YTS):
```bash
http_proxy=http://127.0.0.1:3128 ./hgmcmd proxy 127.0.0.1 8080
```

The proxy will use ./_aliases as its json-storage directory (fixme: this will change)

Mounting the filesystem via FUSE is also possible. Just run

```bash
./hgmcmd mount /mnt/hgms
```

Note that you need to keep the proxy running while the filesystem is mounted.

You can also cross compile the binary for android, see [README.android](https://github.com/adrian-bl/hyperglobalmegastore/blob/master/README.android) for details.


How to upload 'pictures'
----------------------------------------------

Create your flickr account and configure Flickr::Upload:

1. Run 'flickr_upload --auth' and copy-n-paste the URL into your browser (you should already be logged in to flickr)
2. Authorize Flickr::Uploader
3. Hit ENTER and copy the auth_token
4. Create a ~/.flickrrc file with auth_key and url_prefix (= the URL of your account)
The file should look like this:
```
auth_token=12345678901234567-aabbccddeeffaabb
url_prefix=https://www.flickr.com/photos/721538313@N05
```


Afterwards run

```bash
find whatever/ | ./mkpng.pl
```

to encrypt and upload your files.
mkpng.pl will drop a json file with the image location + encryption key into ./_aliases

# RPM Add Checksum

A fundamental tool to compute and surgically add SHA256 hashes to the header of
an rpm file to enable FIPS compatibility.  No other bytes in the rpm file are
changed, so the updated and original RPM functions are exactly the same. 

While this tool enhances the digest/checksums inside an existing RPM file, it
should only be used when a chain of custody can be verified out-of-band.  The
intent of enabling FIPS is to ensure that a higher standard of digest checks
are done and thus a higher level of confidence that the files have not been
tampered.  Adding a checksum with this tool adds this confidence-- act wisely
as with great power comes great responsibility.

```bash
$ ./rpm-add-checksum -h
rpm-add-checksum,  Version: 0.1.20220526.2240 (https://github.com/pschou/rpm-add-checksum)

Usage: rpm-add-checksum [options] input.rpm output.rpm

  -f    Force new file to be written, even if checksum fails
  -i    Do upgrade of file hashes in place
  -t    Test if SHA256 is present in input file
  -v    Turn on verbose
```

## Examples

Adding a sha256 header to a file:
```bash
$ ./rpm-add-checksum -v -i hello.rpm
reading package to hash
Finished hashing package:
  f5a36aea3bfdf2206eb8babfc42b1f42606db7c9
  6a3a905fab31fe49105203044b55124ef870489aef7028a6abbb3e76338ee55c
Payload header:
  adding 6a3a905fab31fe49105203044b55124ef870489aef7028a6abbb3e76338ee55c
Computing new header hash:
  sha1: 4a8dc6e3015afaade19cca47b61caf1a5b4ffbbe
  sha256: cf9ecbf0c0389948a463811b9ef3ebdf1a64314c03c1277b242e0d729816d652
  md5: _XiGKKOepeIaKm5GNHjUrQ==
  original md5: vA7he-bCNai-4zosehFhQg==
Signature headers:
  updated sha1, prev: e90be5a077c965790392fb78364f2354c9bf9884
  added sha256
  Updated size, prev: [4130] now 4234 = 260 2640 1594
  Updated md5, prev: vA7he-bCNai-4zosehFhQg== now _XiGKKOepeIaKm5GNHjUrQ==
```

File already has sha256 header:
```bash
$ ./rpm-add-checksum -v -i hello.rpm
Found Digest SHA256
```

When the internal checksum fails, the sha256 header upgrade is not done:
```bash
$ ./rpm-add-checksum -v -i  hello.rpm
reading package to hash
Finished hashing package:
  c19346950f20d5051373982115daf833aa2a993c
  b8fa954a8a61796b288d54a4e154ffb68688f7abfb1d3eb4eb9f9647e4ccacde
Payload header:
  adding b8fa954a8a61796b288d54a4e154ffb68688f7abfb1d3eb4eb9f9647e4ccacde
Computing new header hash:
  sha1: 0efbf5fc728638ef90d675564406d9286fb5db4e
  sha256: dcb2e8a128e192d23ac7bfc1401012ca48d39a07dd7f386041e59a3294e471c5
  md5: hDTjiJ8PQ8JmCmMv_X3BtQ==
  original md5: 9jkEXouLBiMWxDunhFvgvg==
Signature headers:
  updated sha1, prev: e90be5a077c965790392fb78364f2354c9bf9884
  added sha256
  Updated size, prev: [4130] now 4234 = 260 2640 1594
  Updated md5, prev: vA7he-bCNai-4zosehFhQg== now hDTjiJ8PQ8JmCmMv_X3BtQ==
Error verifying internal MD5 checksum--
```

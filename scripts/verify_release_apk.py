"""Verify release APK identity, version and its privately provisioned signing identity."""
import argparse
import os
from pathlib import Path
import re
import subprocess


def validate(badging, signatures, version, code, expected_cert):
    match = re.search(r"^package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'", badging, re.M)
    if not match or match.groups() != ('dev.modex', str(code), version):
        raise ValueError('APK package/version does not match release')
    certs = re.findall(r'^Signer #\d+ certificate SHA-256 digest: ([0-9a-fA-F]+)$', signatures, re.M)
    expected = expected_cert.replace(':', '').lower()
    if not re.fullmatch('[0-9a-f]{64}', expected) or [c.lower() for c in certs] != [expected]:
        raise ValueError('APK signing certificate does not match configured release identity')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('apk', type=Path)
    parser.add_argument('--version', required=True)
    parser.add_argument('--code', required=True, type=int)
    parser.add_argument('--build-tools', required=True, type=Path)
    args = parser.parse_args()
    badging = subprocess.check_output([str(args.build_tools/'aapt'), 'dump', 'badging', str(args.apk)], text=True)
    signatures = subprocess.check_output([str(args.build_tools/'apksigner'), 'verify', '--verbose', '--print-certs', str(args.apk)], text=True)
    validate(badging, signatures, args.version, args.code, os.environ['MODEX_CERT_SHA256'])
    print('Verified release APK package, version and signing certificate')

if __name__ == '__main__':
    main()

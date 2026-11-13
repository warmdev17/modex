import unittest
from verify_release_apk import validate

class ReleaseAPKTest(unittest.TestCase):
    def test_identity(self):
        data = "package: name='dev.modex' versionCode='200' versionName='0.1.0-alpha.1'"
        cert = 'ab'*32
        signatures = 'Signer #1 certificate SHA-256 digest: '+cert
        validate(data, signatures, '0.1.0-alpha.1', 200, cert)
        for badging, sig, version, code, expected in [
            (data.replace('dev.modex', 'another.app'), signatures, '0.1.0-alpha.1', 200, cert),
            (data, signatures, '0.1.0', 200, cert),
            (data, signatures, '0.1.0-alpha.1', 201, cert),
            (data, signatures, '0.1.0-alpha.1', 200, 'cd'*32),
            (data, '', '0.1.0-alpha.1', 200, cert),
        ]:
            with self.assertRaises(ValueError): validate(badging, sig, version, code, expected)

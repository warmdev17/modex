import unittest
from check_release_code import check

class ReleaseCodeTest(unittest.TestCase):
    def test_prevents_duplicate_and_out_of_order_release(self):
        releases = [{'tag_name': 'v0.1.0', 'draft': True, 'assets': [{'name': 'android-version-code.txt', 'id': 7}]}]
        check('v0.2.0', 102, releases, lambda _: '101\n')
        for tag, code in [('v0.1.0', 102), ('v0.2.0', 101), ('v0.2.0', 100), ('v0.2.0', 1)]:
            with self.assertRaises(ValueError): check(tag, code, releases, lambda _: '101\n')
        with self.assertRaises(ValueError): check('v0.2.0', 102, releases, lambda _: 'invalid')

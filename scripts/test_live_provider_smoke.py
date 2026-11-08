import unittest
from live_provider_smoke import is_test_command


class CommandAllowlistTests(unittest.TestCase):
    def test_accepts_exact_printf_and_known_shell_wrapper(self):
        for command in ["printf MODEX_APPROVAL_OK", "/usr/sbin/bash -lc 'printf MODEX_APPROVAL_OK'", "/bin/bash -c 'printf MODEX_APPROVAL_OK'"]:
            with self.subTest(command=command):
                self.assertTrue(is_test_command(command))

    def test_rejects_extra_execution_and_shell_flags(self):
        for command in [None, "", "printf MODEX_APPROVAL_OK; touch /tmp/pwned", "printf MODEX_APPROVAL_OK > output", "printf $(whoami)", "/bin/bash -c 'printf MODEX_APPROVAL_OK; id'", "/bin/bash --rcfile injected -c 'printf MODEX_APPROVAL_OK'", "evil/bash -lc 'printf MODEX_APPROVAL_OK'", "printf MODEX_APPROVAL_OK && id", "printf 'MODEX_APPROVAL_OK", "sh -c 'printf MODEX_APPROVAL_OK'", "printf MODEX_APPROVAL_OK\nid", "printf MODEX_APPROVAL_OK | cat"]:
            with self.subTest(command=command):
                self.assertFalse(is_test_command(command))


if __name__ == "__main__":
    unittest.main()

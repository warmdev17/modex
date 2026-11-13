"""Check the release matrix and archive contents without executing foreign binaries."""
from pathlib import Path
import argparse
import tarfile
import zipfile
import subprocess


def verify(root):
    for platform in ('linux', 'darwin', 'windows'):
        for arch in ('amd64', 'arm64'):
            extension = 'zip' if platform == 'windows' else 'tar.gz'
            matches = list(root.glob(f'modex_*_{platform}_{arch}.{extension}'))
            if len(matches) != 1:
                raise ValueError(f'missing or duplicate {platform}/{arch} archive')
            if extension == 'zip':
                with zipfile.ZipFile(matches[0]) as archive: names = set(archive.namelist())
                if 'modex.exe' not in names or 'modex-host.exe' in names:
                    raise ValueError('Windows archive must contain SSH client only')
            else:
                with tarfile.open(matches[0]) as archive:
                    for binary in ('modex', 'modex-host'):
                        item = archive.getmember(binary)
                        if not item.isfile() or not item.mode & 0o111:
                            raise ValueError('archive lacks executable sibling binaries')
    for arch in ('amd64', 'arm64'):
        for extension in ('deb', 'rpm'):
            matches = list(root.glob(f'modex_*_{arch}.{extension}'))
            if len(matches) != 1: raise ValueError(f'missing {arch} {extension}')
            if extension == 'deb':
                listing = subprocess.check_output(['dpkg-deb', '-c', str(matches[0])], text=True)
                if './usr/bin/modex\n' not in listing or './usr/bin/modex-host\n' not in listing:
                    raise ValueError('DEB lacks sibling binaries')
    print('Verified six native archives and four Linux packages')

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('directory', type=Path)
    verify(parser.parse_args().directory)

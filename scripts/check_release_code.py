"""Refuse re-publishing a tag or lowering Android versionCode, including draft releases."""
import argparse
import json
import subprocess


def check(tag, code, releases, read_asset):
    if code < 2 or code > 2100000000:
        raise ValueError('Android versionCode is out of range')
    for release in releases:
        if release['tag_name'] == tag:
            raise ValueError('Release already exists; inspect it manually before retrying')
        for asset in release.get('assets', []):
            if asset['name'] == 'android-version-code.txt':
                previous = int(read_asset(asset['id']).strip())
                if previous >= code:
                    raise ValueError('Android versionCode must exceed all earlier release/draft codes')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--repo', required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--code', type=int, required=True)
    args = parser.parse_args()
    endpoint = f'repos/{args.repo}/releases'
    pages = json.loads(subprocess.check_output(['gh', 'api', '--paginate', '--slurp', endpoint], text=True))
    def read_asset(asset_id):
        return subprocess.check_output(['gh', 'api', f'{endpoint}/assets/{int(asset_id)}', '-H', 'Accept: application/octet-stream'], text=True)
    check(args.tag, args.code, [r for page in pages for r in page], read_asset)
    print('Release tag and Android versionCode are new')

if __name__ == '__main__':
    main()

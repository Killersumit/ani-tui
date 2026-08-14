'use strict';

function releaseTarget(platform = process.platform, arch = process.arch) {
  const platforms = {
    linux: 'linux',
    darwin: 'darwin',
    win32: 'windows',
  };
  const architectures = {
    x64: 'x64',
    arm64: 'arm64',
    arm: 'armv7',
  };

  const releaseOS = platforms[platform];
  const releaseArch = architectures[arch];
  if (!releaseOS || !releaseArch || (releaseOS !== 'linux' && releaseArch === 'armv7')) {
    throw new Error(
      `ani-tui does not publish a binary for ${platform}/${arch}. ` +
      'Supported targets are Linux x64/arm64/armv7, macOS x64/arm64, and Windows x64/arm64.',
    );
  }

  const extension = platform === 'win32' ? '.exe' : '';
  return {
    os: releaseOS,
    arch: releaseArch,
    extension,
    asset: `ani-tui-${releaseOS}-${releaseArch}${extension}`,
  };
}

module.exports = { releaseTarget };

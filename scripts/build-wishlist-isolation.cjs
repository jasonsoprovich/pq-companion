// Bundle the actual page with a transport-only regression fixture.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const repo = path.resolve(__dirname, '..');
const [outputArg, sourceArg = repo] = process.argv.slice(2);
if (!outputArg) throw new Error('Usage: node scripts/build-wishlist-isolation.cjs OUTPUT [SOURCE_REPO]');
const output = path.resolve(outputArg), source = path.resolve(sourceArg);
const esbuild = require(require.resolve('esbuild', { paths: [repo] }));
(async () => {
  fs.mkdirSync(output, { recursive: true });
  const built = await esbuild.build({
    absWorkingDir: repo,
    entryPoints: [path.join(repo, 'frontend/tests/wishlist-isolation.tsx')],
    bundle: true, write: false, platform: 'browser', format: 'iife', metafile: true,
    nodePaths: [path.join(repo, 'node_modules')],
    define: { 'process.env.NODE_ENV': '"production"' },
    plugins: [{ name: 'source-under-test', setup(build) {
      build.onResolve({ filter: /^\.\.\/src\/pages\/WishlistPage$/ }, () => ({ path: path.join(source, 'frontend/src/pages/WishlistPage.tsx') }));
    } }],
  });
  fs.writeFileSync(path.join(output, 'fixture.html'), `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'"><div id="root"></div><pre id="result">PENDING</pre><script>${built.outputFiles[0].text.replace(/<\/script/gi, '<\\/script')}</script>`);
  const inputs = {};
  for (const p of Object.keys(built.metafile.inputs)) {
    const full = path.resolve(repo, p);
    inputs[full] = crypto.createHash('sha256').update(fs.readFileSync(full)).digest('hex');
  }
  fs.writeFileSync(path.join(output, 'input-hashes.json'), JSON.stringify(inputs, null, 2));
  console.log(JSON.stringify({ source, inputCount: Object.keys(inputs).length, page: inputs[path.join(source, 'frontend/src/pages/WishlistPage.tsx')] }));
})().catch(error => { console.error(error); process.exitCode = 1; });

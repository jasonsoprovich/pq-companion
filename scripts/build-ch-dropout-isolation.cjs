const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const repo = path.resolve(__dirname, '..');
const [outputArg, sourceArg = repo] = process.argv.slice(2);
const output = path.resolve(outputArg), source = path.resolve(sourceArg);
const esbuild = require(require.resolve('esbuild', { paths: [repo] }));
(async () => {
  fs.mkdirSync(output, { recursive: true });
  const built = await esbuild.build({
    absWorkingDir: repo, entryPoints: [path.join(repo, 'frontend/tests/ch-dropout-owners.tsx')],
    bundle: true, write: false, platform: 'browser', format: 'iife', metafile: true,
    nodePaths: [path.join(repo, 'node_modules')], define: { 'process.env.NODE_ENV': '"production"' },
    plugins: [{ name: 'source-and-inert-audio-transport', setup(build) {
      build.onResolve({ filter: /^\.\.\/src\// }, args => ({ path: path.join(source, 'frontend/src', args.path.slice(7) + (args.path.endsWith('chMetronome') || args.path.includes('hooks/') ? '.ts' : '.tsx')) }));
      build.onResolve({ filter: /\/services\/audio$/ }, () => ({ path: path.join(repo, 'frontend/tests/ch-dropout-audio.ts') }));
    } }],
  });
  fs.writeFileSync(path.join(output, 'fixture.html'), `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'"><div id="root"></div><pre id="result">PENDING</pre><script>${built.outputFiles[0].text.replace(/<\/script/gi, '<\\/script')}</script>`);
  const inputs = {};
  for (const p of Object.keys(built.metafile.inputs)) {
    const full = path.resolve(repo, p);
    inputs[full] = crypto.createHash('sha256').update(fs.readFileSync(full)).digest('hex');
  }
  fs.writeFileSync(path.join(output, 'input-hashes.json'), JSON.stringify(inputs, null, 2));
  console.log(JSON.stringify({ source, inputCount: Object.keys(inputs).length }));
})().catch(error => { console.error(error); process.exitCode = 1; });

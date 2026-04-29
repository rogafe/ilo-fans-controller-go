const esbuild = require('esbuild');

const isWatch = process.argv.includes('--watch');

/** @type {import('esbuild').BuildOptions} */
const buildOptions = {
  entryPoints: ['./assets/src/main.js'],
  bundle: true,
  outfile: './assets/dist/bundle.js',
  minify: !isWatch,
  sourcemap: isWatch,
  target: ['es2020'],
  format: 'iife',
  platform: 'browser',
  define: {
    'process.env.NODE_ENV': isWatch ? '"development"' : '"production"'
  }
};

if (isWatch) {
  // Watch mode
  esbuild.context(buildOptions).then(context => {
    context.watch();
    console.log('Watching for changes...');
  });
} else {
  // Build mode
  esbuild.build(buildOptions).then(() => {
    console.log('Build complete');
  }).catch(() => process.exit(1));
} 
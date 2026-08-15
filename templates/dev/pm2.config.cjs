const { createHash } = require('node:crypto');
const { basename, resolve } = require('node:path');

const cwd = resolve(__dirname);
const project = basename(cwd).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'robot';
const hash = createHash('sha256').update(cwd).digest('hex').slice(0, 8);

/** @type {{ apps: import("pm2").StartOptions[] }} */
module.exports = {
  apps: [
    {
      name: `alemonx-${project.slice(0, 40)}-${hash}`,
      namespace: 'alemonx',
      cwd,
      script: './index.js',
      autorestart: true,
      min_uptime: '10s',
      max_restarts: 10,
      restart_delay: 3000,
      exp_backoff_restart_delay: 1000,
      env: {
        NODE_ENV: 'production'
      }
    }
  ]
};

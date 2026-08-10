const {execFile} = require('child_process')
const fs = require('fs')
const path = require('path')

function resolveCli({configuredPath = '', env = process.env, existsSync = fs.existsSync} = {}) {
  if (configuredPath) return configuredPath
  const installed = env.LOCALAPPDATA && path.join(env.LOCALAPPDATA, 'Programs', 'oberth', 'oberth.exe')
  return installed && existsSync(installed) ? installed : 'oberth'
}

function createCliAdapter({configuredPath = '', env = process.env, existsSync = fs.existsSync, execute = execFile} = {}) {
  const executable = resolveCli({configuredPath, env, existsSync})
  return {
    executable,
    run(args, cwd) {
      return new Promise((resolve, reject) => {
        execute(executable, args, {cwd, windowsHide: true}, (error, stdout = '', stderr = '') => {
          if (!error) {
            resolve(stdout.trim())
            return
          }
          if (error.code === 'ENOENT') {
            reject(new Error(`Oberth CLI was not found at "${executable}". Configure oberth.cliPath or install the CLI.`))
            return
          }
          reject(new Error((stderr || error.message || 'Oberth CLI command failed.').trim()))
        })
      })
    },
  }
}

module.exports = {createCliAdapter, resolveCli}

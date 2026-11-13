import { expect, test } from '@playwright/test'

test('primary navigation stays reachable while switching sections', async ({
  page
}) => {
  page.on('pageerror', error => console.error(error.message))
  await page.route('**/project-navigation-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root" style="width:240px;margin-left:auto"></div><script type="module">
      import '/src/styles.css';
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const { default: React } = await import('/node_modules/.vite/deps/react.js');
      const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
      const { ProjectNavigation } = await import('/src/components/ProjectNavigation.tsx');
      let active = 'runtime';
      let section = 'robot';
      const root = ReactDOM.createRoot(document.getElementById('root'));
      const render = () => root.render(React.createElement(Fixture));
      const setActive = value => { active = value; render(); };
      const setSection = value => { section = value; render(); };
      function Fixture() {
        return React.createElement(ProjectNavigation, {activeID: active, items: [
          {id:'runtime', label:'运行', onSelect: () => setActive('runtime')},
          {id:'config', label:'配置', onSelect: () => {setActive('config');setSection('robot')}, children: [
            {id:'robot', label:'机器人配置', active:active === 'config' && section === 'robot', onSelect: () => {setActive('config');setSection('robot')}},
            {id:'env', label:'环境变量', active:active === 'config' && section === 'env', onSelect: () => {setActive('config');setSection('env')}}
          ]}
        ]});
      }
      render();
    </script>`
    })
  )
  await page.goto('/project-navigation-regression')
  const running = page.getByRole('button', { name: '运行', exact: true })
  await expect(running).toHaveAttribute('aria-current', 'page')
  const config = page.getByRole('button', { name: '配置', exact: true })
  const nav = page.getByRole('navigation', { name: '机器人功能' })
  const menu = page.getByRole('group', { name: '配置子菜单' })
  const before = (await nav.boundingBox())!
  await config.click()
  await expect(menu).toBeVisible()
  expect((await nav.boundingBox())!.height).toBe(before.height)
  const menuBounds = (await menu.boundingBox())!
  expect(menuBounds.x + menuBounds.width).toBeLessThan(before.x)
  await page.getByRole('button', { name: '环境变量', exact: true }).click()
  await expect(menu).toHaveCount(0)
  await expect(config).toHaveAttribute('aria-current', 'page')
  await config.click()
  await expect(menu.getByRole('button', { name: '环境变量', exact: true }))
    .toHaveAttribute('aria-current', 'page')
  await config.press('Escape')
  await expect(menu).toHaveCount(0)
  await running.click()
  await expect(running).toHaveAttribute('aria-current', 'page')
  await expect(config).toBeVisible()
})

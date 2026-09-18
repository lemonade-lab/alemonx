import { expect, test } from '@playwright/test'

test('primary navigation stays reachable while switching sections', async ({
  page
}) => {
  page.on('pageerror', error => console.error(error.message))
  await page.route('**/project-navigation-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
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
            {id:'robot', label:'机器人配置', active:section === 'robot', onSelect: () => setSection('robot')},
            {id:'env', label:'环境变量', active:section === 'env', onSelect: () => setSection('env')}
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
  await page.getByRole('button', { name: '配置', exact: true }).click()
  await expect(running).toBeVisible()
  await expect(
    page.getByRole('button', { name: '机器人配置', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await page.getByRole('button', { name: '环境变量', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '环境变量', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await running.click()
  await expect(running).toHaveAttribute('aria-current', 'page')
  await expect(
    page.getByRole('button', { name: '环境变量', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '配置', exact: true })
  ).toBeVisible()
})

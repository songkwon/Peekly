import { useState } from 'react'
import { ArrowRight, Check, Clipboard, Globe2, Monitor, Radio, Terminal } from 'lucide-react'
import { SelectField, selectOptions } from './SelectField'

export function ConnectionGuide({ address, host, running, platform, portDraft, setPortDraft, savePort, savedPort, automaticActive, captureMode, setCaptureMode, appearance, setAppearance, copy }: {
  address: string, host: string, running: boolean, platform: string, portDraft: string, setPortDraft: (value: string) => void,
  savePort: () => void, savedPort: number, automaticActive: boolean, captureMode: 'automatic' | 'manual' | null,
  setCaptureMode: (mode: 'automatic' | 'manual' | null) => void, appearance: 'system' | 'light' | 'dark',
  setAppearance: (value: 'system' | 'light' | 'dark') => void, copy: (value: string) => void
}) {
  const [guide, setGuide] = useState<'system' | 'terminal' | 'device'>('system')
  const [device, setDevice] = useState<'ios' | 'android' | 'simulator' | 'emulator'>('ios')
  const windows = platform === 'windows'
  const listeningPort = running ? Number(address.slice(address.lastIndexOf(':') + 1)) : savedPort
  const physicalDevice = device === 'ios' || device === 'android'
  const simulator = device === 'simulator'
  const localOnly = host === '127.0.0.1'
  const deviceProxy = device === 'emulator' && localOnly ? `10.0.2.2:${listeningPort}` : `${host}:${listeningPort}`
  const deviceCheck = 'http://peekly.test/'
  const simulatorCertificate = 'xcrun simctl keychain booted add-root-cert /path/to/Peekly.cer'
  const usingAlternatePort = running && listeningPort !== savedPort
  const command = `${windows ? 'curl.exe' : 'curl'} --proxy http://${address} http://example.com/`
  const guides = [{ id: 'system', title: '系统代理', icon: Monitor }, { id: 'terminal', title: '终端', icon: Terminal }, { id: 'device', title: '移动端', icon: Globe2 }] as const
  return <div className="guide-content">
    <div className="guide-endpoint">
      <div className="guide-endpoint-icon"><Radio size={20} /></div>
      <div className="guide-endpoint-address"><small>HTTP 代理地址</small><strong className="mono">{address}</strong></div>
      <span className={`guide-state ${running ? 'running' : ''}`}><i />{running ? '正在监听' : '未启动'}</span>
      <button className="btn subtle-button" onClick={() => copy(address)}><Clipboard size={14} /> 复制地址</button>
    </div>

    <div className="guide-layout">
      <div className="guide-method">
        <div className="guide-tabs" aria-label="连接方式">
          {guides.map(({ id, title, icon: Icon }) => <button type="button" key={id} aria-pressed={guide === id} onClick={() => setGuide(id)}><Icon size={14} /> {title}</button>)}
        </div>
        {guide === 'system' && automaticActive ? <div className="guide-copy">
          <strong>系统代理已接入 Peekly</strong><p>刷新浏览器即可看到新请求。停止抓包会恢复启动前的代理设置。</p>
        </div> : guide === 'system' && captureMode !== 'manual' ? <div className="guide-copy">
          <strong>自动接入浏览器等应用</strong><p>点击「开始抓包」，选择「使用系统代理」，再刷新浏览器。停止抓包会恢复原系统设置。</p>
          <p>若目标应用使用自己的代理设置，可在该应用中填写上方代理地址。</p>
        </div> : guide === 'system' ? <ol className="guide-steps">
          <li><span>{running ? <Check size={12} /> : '1'}</span><div><strong>启动抓包</strong><p>在工具栏选择「开始抓包」。</p></div></li>
          <li><span>2</span><div><strong>打开{windows ? ' Windows ' : ' macOS '}代理设置</strong>
            <p>{windows ? '设置 → 网络和 Internet → 代理 → 手动设置代理' : '系统设置 → 网络 → 当前网络 → 详细信息 → 代理'}</p></div></li>
          <li><span>3</span><div><strong>填写代理地址</strong>
            <p>{windows ? `将服务器设为 ${host}，端口设为 ${savedPort}。` : `开启 HTTP 与 HTTPS 网页代理，服务地址设为 ${host}，端口设为 ${savedPort}。`}</p></div></li>
        </ol> : guide === 'terminal' ? <div className="guide-copy">
          <strong>发送一条测试请求</strong><p>启动抓包后，在终端执行此命令。请求会出现在流量列表。</p>
          <button className="command-block mono" onClick={() => copy(command)}><code>{command}</code><Clipboard size={14} /></button>
        </div> : <div className="mobile-setup">
          <div className="device-tabs" aria-label="移动端平台">
            {([['ios', 'iPhone / iPad'], ['android', 'Android'], ['simulator', 'iOS 模拟器'], ['emulator', 'Android 模拟器']] as const).map(([id, label]) =>
              <button type="button" key={id} className="btn subtle-button" aria-pressed={device === id} onClick={() => setDevice(id)}>{label}</button>)}
          </div>
          {simulator && windows ? <p className="guide-footnote">iOS 模拟器需要在 Mac 上运行。</p> : <>
            <ol className="guide-steps">
              <li><span>1</span><div><strong>启动抓包</strong>
                <p>{physicalDevice ? '手机和电脑连同一 Wi-Fi。在 Peekly 的「网络设置」选择电脑的局域网 IP，允许手机 IP，再以手动模式开始抓包。'
                  : simulator ? '启动 iOS 模拟器，再在 Peekly 中选择「使用系统代理」。'
                  : '启动 Android Studio 模拟器，并在 Peekly 中开始抓包。'}</p>
              </div></li>
              <li><span>2</span><div><strong>{simulator ? '检查连接' : '设置设备代理'}</strong>
                {!simulator && <p>{device === 'ios' ? '在设备上打开「设置 → Wi-Fi → 当前网络 → 配置代理 → 手动」。'
                  : '在 Android 的「设置 → Wi-Fi → 当前网络」中，把代理设为手动。'}</p>}
                {!simulator && (physicalDevice && localOnly ? <p>请先将监听地址从 127.0.0.1 改为局域网 IP。</p> :
                  <button className="command-block mono" type="button" onClick={() => copy(deviceProxy)}>
                    <code>{deviceProxy}</code><Clipboard size={13} />
                  </button>)}
                <p>代理设好后，在设备浏览器输入以下 HTTP 地址。出现「已通过 Peekly 连接」即成功。</p>
                <button className="command-block mono" type="button" onClick={() => copy(deviceCheck)}><code>{deviceCheck}</code><Clipboard size={13} /></button>
              </div></li>
            </ol>
            <details>
              <summary>查看 HTTPS 正文时，再安装证书</summary>
              <p>{device === 'ios' ? '从连接检查页下载证书，在「设置 → 已下载的描述文件」安装。再到「通用 → 关于本机 → 证书信任设置」开启完全信任。'
                : simulator ? '从 Peekly 导出根证书，用以下命令将它加入已启动的模拟器。'
                : '从连接检查页下载证书，在「设置 → 安全 → 加密与凭据 → 安装证书」中选 CA 证书。自行开发的 App 可在调试配置中信任用户 CA。'}</p>
              {simulator && <button className="command-block mono" type="button" onClick={() => copy(simulatorCertificate)}>
                <code>{simulatorCertificate}</code><Clipboard size={13} />
              </button>}
              <p>安装后，在 Peekly 的「HTTPS 解密」中启用解密。抓包结束后关闭设备代理。</p>
            </details>
            <details>
              <summary>打不开连接检查页</summary>
              <p>这个地址由 Peekly 代理提供。确认已启动抓包、设备已使用 Peekly 代理，并保留地址开头的 http://。</p>
            </details>
          </>}
        </div>}
        {guide === 'system' && !automaticActive && captureMode === 'manual' && <p className="guide-footnote"><ArrowRight size={12} /> 抓包结束后，撤销为 Peekly 添加的系统代理设置。</p>}
      </div>

      <div className="guide-settings">
        <h3>应用设置</h3>
        <label htmlFor="proxy-port">监听端口</label>
        <div className="guide-input-row">
          <input className="input text-input mono" type="text" inputMode="numeric" pattern="[0-9]{4,5}" maxLength={5} id="proxy-port" disabled={running} value={portDraft} onChange={event => setPortDraft(event.target.value.replace(/\D/g, ''))} onKeyDown={event => {
            if (event.key === 'Enter' && portDraft !== String(savedPort)) { event.preventDefault(); savePort() }
          }} />
          <button className="btn subtle-button" disabled={running || portDraft === String(savedPort)} onClick={savePort}>保存</button>
        </div>
        {running && <small>{usingAlternatePort ? `首选端口被占用，本次使用 ${listeningPort}。` : '停止抓包后可修改端口'}</small>}
        <label htmlFor="capture-mode">抓包方式</label>
        <SelectField id="capture-mode" disabled={running} value={captureMode ?? 'ask'}
          options={selectOptions({ ask: '启动时询问', automatic: '自动配置系统代理', manual: '手动连接' })}
          onChange={mode => setCaptureMode(mode === 'automatic' || mode === 'manual' ? mode : null)} />
        <label htmlFor="appearance">外观</label>
        <SelectField id="appearance" value={appearance} onChange={setAppearance}
          options={[{ value: 'system', label: '跟随系统' }, { value: 'light', label: '浅色' }, { value: 'dark', label: '深色' }]} />
      </div>
    </div>
  </div>
}

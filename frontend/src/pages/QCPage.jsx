import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api.js'
import { Badge, BATCH_STATUS, Empty } from '../components.jsx'

export default function QCPage() {
  const [d, setD] = useState(null)
  const nav = useNavigate()
  useEffect(() => { api('/qc/overview').then(setD) }, [])
  if (!d) return <div className="muted">加载中…</div>

  return (
    <div>
      <h1>品控看板</h1>

      <h2 style={{ margin: '6px 0 10px' }}>批次口味/设备反馈聚合与停售建议</h2>
      <div className="panel">
        <table>
          <thead><tr>
            <th>批次</th><th>酒款</th><th>反馈酒吧</th><th>口味异常</th><th>气压问题</th><th>出酒问题</th><th>客诉</th><th>状态</th><th>系统建议</th>
          </tr></thead>
          <tbody>
            {d.batch_alerts.map(b => (
              <tr key={b.batch_id}>
                <td><b>{b.batch_no}</b></td>
                <td>{b.beer}（{b.style}）</td>
                <td>{b.bars >= 2 ? <span className="badge red">{b.bars} 家</span> : `${b.bars} 家`}</td>
                <td>{b.taste ? <span className="badge red">{b.taste}</span> : 0}</td>
                <td>{b.pressure_issues}</td>
                <td>{b.speed_issues}</td>
                <td>{b.complaints}</td>
                <td><Badge map={BATCH_STATUS} value={b.status} /></td>
                <td>
                  <div className={'suggest ' + (b.suggest_suspend ? 'danger' : '')} style={{ display: 'inline-block' }}>
                    {b.suggestion}
                  </div>
                  {b.suggest_suspend && b.status === 'NORMAL' &&
                    <button className="small danger" style={{ marginLeft: 8 }}
                      onClick={() => nav('/batches')}>去停售/召回</button>}
                </td>
              </tr>
            ))}
            {d.batch_alerts.length === 0 && <tr><td colSpan={9}><Empty text="暂无反馈" /></td></tr>}
          </tbody>
        </table>
      </div>

      <div className="grid cols-2">
        <AbnormalPanel title="清洗不合格桶" data={d.wash_fails} reason />
        <AbnormalPanel title="酒桶丢失" data={d.lost} reason />
        <AbnormalPanel title="桶体变形" data={d.deformed} reason />
        <AbnormalPanel title="司机漏扫·桶去向待核查" data={d.missing_scan} reason />
      </div>

      <h2 style={{ margin: '14px 0 10px' }}>开桶反馈流（最近 30 条）</h2>
      <div className="tablewrap panel" style={{ padding: 0, maxHeight: 320 }}>
        <table>
          <thead><tr><th>时间</th><th>酒吧</th><th>批次</th><th>出酒</th><th>气压</th><th>口味</th><th>客诉</th></tr></thead>
          <tbody>
            {d.feedbacks.map(f => (
              <tr key={f.id}>
                <td className="small muted">{f.at}</td><td>{f.bar}</td>
                <td className="small">{f.style} {f.batch_no}</td>
                <td>{f.pour_speed || '—'}</td><td>{f.pressure || '—'}</td>
                <td>{f.taste_off ? <span className="badge red">{f.taste_note || '异常'}</span> : '正常'}</td>
                <td className="small">{f.complaint || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function AbnormalPanel({ title, data }) {
  return (
    <div className="panel">
      <h3>{title} <span className="badge red">{data.length}</span></h3>
      <table>
        <tbody>
          {data.map((x, i) => (
            <tr key={i}>
              <td><code className="kcode">{x.keg_code}</code></td>
              <td className="small">{x.bar}</td>
              <td className="small muted">{x.reason}</td>
            </tr>
          ))}
          {data.length === 0 && <tr><td><Empty text="无" /></td></tr>}
        </tbody>
      </table>
    </div>
  )
}

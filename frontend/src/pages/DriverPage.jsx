import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { Badge, RES_STATUS, useToast, Empty, VISIT } from '../components.jsx'

const VISIT_CLS = { READY: 'green', BAR_NOT_READY: 'red', DRIVER_FAILED: 'purple' }

export default function DriverPage() {
  const [tasks, setTasks] = useState([])
  const [audit, setAudit] = useState(null)
  const toast = useToast()
  const load = () => api('/driver/tasks').then(d => setTasks(d.tasks))
  useEffect(load, [])

  const runAudit = async () => {
    try {
      const d = await api('/driver/audit', { method: 'POST', body: {} })
      setAudit(d)
      toast(`审计完成：漏扫 ${d.missing_scans.length} 只，爽约 ${d.missed_reservations} 单，新增逾期 ${d.new_late_returns} 只`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 12 }}>
        <h1 style={{ margin: 0 }}>空桶回收扫码</h1>
        <span className="spacer" />
        <button className="danger" onClick={runAudit}>执行回收审计（漏扫/爽约/逾期）</button>
      </div>
      <div className="muted small" style={{ marginBottom: 12 }}>
        到店后请先登记到场结果再扫码：<b>酒吧未备好空桶</b>计入酒吧方逾期责任；<b>司机到店失败</b>（车辆/调度等）作为酒吧方免责依据，二者分开记录。
      </div>
      {audit && (audit.missing_scans.length > 0 || audit.missed_reservations > 0) && (
        <div className="panel" style={{ borderColor: '#e0b3aa', background: '#fdf3f1' }}>
          <h3>审计结果</h3>
          {audit.missing_scans.length > 0 && <div className="small">
            <b>司机漏扫（预约已收但无扫码记录）：</b>
            {audit.missing_scans.map((m, i) =>
              <span key={i} className="badge red" style={{ margin: '3px 6px 3px 0' }}>
                {m.keg_code} @ {m.bar}（预约 #{m.reservation_id}）
              </span>)}
          </div>}
          {audit.missed_reservations > 0 && <div className="small" style={{ marginTop: 6 }}>爽约预约 {audit.missed_reservations} 单，已标记延迟归还。</div>}
          {audit.new_late_returns > 0 && <div className="small" style={{ marginTop: 6 }}>新触发账期逾期 {audit.new_late_returns} 只。</div>}
        </div>
      )}

      {tasks.map(t => <TaskCard key={t.reservation_id} t={t} onChange={load} />)}
      {tasks.length === 0 && <div className="panel"><Empty text="当前城市暂无回收任务" /></div>}
    </div>
  )
}

function TaskCard({ t, onChange }) {
  const toast = useToast()
  const [code, setCode] = useState('')
  const [appearance, setAppearance] = useState('完好')
  const [residue, setResidue] = useState('无')
  const [cap, setCap] = useState('已封盖')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)

  const scan = async () => {
    if (!code.trim()) { toast('请输入/扫描桶编号', 'err'); return }
    setBusy(true)
    try {
      const d = await api('/pickups/scan', {
        method: 'POST',
        body: { reservation_id: t.reservation_id, keg_code: code.trim().toUpperCase(), appearance, residue_level: residue, cap_status: cap }
      })
      toast(`已收回 ${code.toUpperCase()}${d.deformed ? '，桶体变形已标记' : ''}`)
      setCode('')
      onChange()
    } catch (e) { toast(e.message, 'err') } finally { setBusy(false) }
  }

  // 登记到店结果（整单；可在备注写明个别桶情况）
  const visit = async (result) => {
    setBusy(true)
    try {
      await api('/driver/visit', { method: 'POST', body: { reservation_id: t.reservation_id, result, note } })
      toast({ READY: '已登记到店且空桶备好', BAR_NOT_READY: '已登记：酒吧未备好空桶（计入酒吧责任）', DRIVER_FAILED: '已登记：司机到店失败（酒吧免责依据）' }[result])
      setNote('')
      onChange()
    } catch (e) { toast(e.message, 'err') } finally { setBusy(false) }
  }

  const allScanned = t.cycles.length > 0 && t.scanned_count === t.cycles.length
  const hasVisit = (t.visit_ready || 0) + (t.visit_not_ready || 0) + (t.visit_failed || 0) > 0

  return (
    <div className="panel">
      <div className="row">
        <div>
          <h2 style={{ margin: 0 }}>{t.bar} <span className="muted small">{t.city}</span></h2>
          <div className="small muted">预约日 {t.pickup_date} · 应收 {t.empty_count} 只 · {t.note || '无备注'}</div>
        </div>
        <span className="spacer" />
        <Badge map={RES_STATUS} value={t.status} />
        {t.visit_not_ready > 0 && <span className="badge red">到店未备货 {t.visit_not_ready}</span>}
        {t.visit_failed > 0 && <span className="badge purple">到店失败 {t.visit_failed}</span>}
        {t.missing_count > 0 && <span className="badge red">漏扫 {t.missing_count} 只</span>}
        {allScanned && <span className="badge green">已全部扫码</span>}
      </div>

      <div className="tablewrap" style={{ maxHeight: 170, marginTop: 10 }}>
        <table>
          <thead><tr><th>桶编号</th><th>阶段</th><th>到场记录</th><th>扫码状态</th></tr></thead>
          <tbody>
            {t.cycles.map(c => (
              <tr key={c.cycle_id}>
                <td><code className="kcode">{c.keg_code}</code>{c.frozen && <span className="badge red" style={{ marginLeft: 4 }}>押金冻结</span>}</td>
                <td className="small">{c.stage}</td>
                <td>{c.visit ? <span className={'badge ' + (VISIT_CLS[c.visit] || 'gray')}>{VISIT[c.visit]?.[0] || c.visit}</span>
                  : <span className="muted small">未登记</span>}</td>
                <td>{c.scanned
                  ? <span className="badge green">已扫码回收</span>
                  : <span className="badge red">未扫码</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {!allScanned && t.status !== 'CANCELLED' && (
        <>
          <div className="grid cols-4" style={{ marginTop: 12, alignItems: 'flex-end' }}>
            <div><label>扫码/输入桶编号</label>
              <input value={code} onChange={e => setCode(e.target.value)} placeholder="KEG-0003"
                onKeyDown={e => e.key === 'Enter' && scan()} autoFocus /></div>
            <div><label>外观</label>
              <select value={appearance} onChange={e => setAppearance(e.target.value)}>
                <option>完好</option><option>凹陷</option><option>变形</option><option>锈蚀</option>
              </select></div>
            <div><label>残液</label>
              <select value={residue} onChange={e => setResidue(e.target.value)}>
                <option>无</option><option>少量</option><option>明显残液</option>
              </select></div>
            <div><label>封盖</label>
              <select value={cap} onChange={e => setCap(e.target.value)}>
                <option>已封盖</option><option>未封盖</option><option>封盖损坏</option>
              </select></div>
          </div>
          <div style={{ marginTop: 8 }}><label>到场情况备注（未备货/到店失败原因）</label>
            <input value={note} onChange={e => setNote(e.target.value)} placeholder="如：桶还在酒头上 / 车辆故障" /></div>
          <div className="row" style={{ marginTop: 10 }}>
            <button className="ghost" disabled={busy || hasVisit} onClick={() => visit('READY')}>① 到店·空桶已备好</button>
            <button className="danger" disabled={busy || hasVisit} onClick={() => visit('BAR_NOT_READY')}>② 到店·酒吧未备货</button>
            <button style={{ background: 'var(--purple,#7c5cbf)', borderColor: 'var(--purple,#7c5cbf)' }} disabled={busy || hasVisit}
              onClick={() => visit('DRIVER_FAILED')}>③ 司机到店失败（改期）</button>
            <span className="spacer" />
            <button disabled={busy} onClick={scan}>确认空数量并扫码回收</button>
          </div>
          {hasVisit && <div className="small muted" style={{ marginTop: 6 }}>本预约已登记到场结果，到场记录已计入逾期责任判定，可继续逐桶扫码。</div>}
        </>
      )}
    </div>
  )
}

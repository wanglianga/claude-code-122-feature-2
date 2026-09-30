import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { Badge, STAGE, BATCH_STATUS, Modal, useToast, Flags, Empty, LIABILITY, fileToDataURL } from '../components.jsx'

const ALERT_KIND = {
  OVERDUE_FREEZE: ['押金冻结', 'red'], RETURN_REMINDER: ['回收提醒', 'amber'],
  LIABILITY_RESULT: ['责任结果', 'purple'], PHOTO_REVIEWED: ['照片核实', 'blue']
}

export default function BarPage() {
  const [cycles, setCycles] = useState([])
  const [barId, setBarId] = useState(null)
  const [alerts, setAlerts] = useState([])
  const [fb, setFb] = useState(null)
  const [sel, setSel] = useState([])           // 普通空桶预约
  const [frozenSel, setFrozenSel] = useState([]) // 逾期冻结桶确认回收时间
  const [reserveOpen, setReserveOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [photoTarget, setPhotoTarget] = useState(null)
  const toast = useToast()

  const load = () => {
    api('/bar/cycles').then(d => { setCycles(d.cycles); setBarId(d.bar_id) })
    api('/bar/alerts').then(d => setAlerts(d.alerts || []))
  }
  useEffect(load, [])

  const toggle = (c) => {
    const bucket = c.deposit_frozen ? frozenSel : sel
    const set = c.deposit_frozen ? setFrozenSel : setSel
    set(bucket.includes(c.cycle_id) ? bucket.filter(v => v !== c.cycle_id) : [...bucket, c.cycle_id])
  }
  const ackAlert = async (id) => {
    try { await api('/bar/alerts/ack/' + id, { method: 'POST' }); load() } catch (e) { toast(e.message, 'err') }
  }

  const frozenN = cycles.filter(c => c.deposit_frozen).length
  const unacked = alerts.filter(a => !a.acked)

  return (
    <div>
      <h1>我的在店酒桶</h1>
      <div className="muted small" style={{ marginBottom: 12 }}>
        开桶后请反馈出酒速度、气压、口味异常与客诉；打空后预约回收。空桶超过账期未归还时平台将按桶冻结押金，
        冻结后请上传“已打空但未回收”现场照片并确认回收时间；司机到店失败与酒吧未备好空桶会分别影响逾期责任。
      </div>

      {unacked.length > 0 && (
        <div className="panel" style={{ borderColor: '#e0b3aa', background: '#fdf3f1', marginBottom: 12 }}>
          <h3 style={{ marginTop: 0 }}>逾期押金 / 回收提醒（{unacked.length}）</h3>
          {unacked.map(a => (
            <div key={a.id} className="notif" style={{ background: '#fff' }}>
              <div className="row">
                <span className={'badge ' + (ALERT_KIND[a.kind]?.[1] || 'gray')}>{ALERT_KIND[a.kind]?.[0] || a.kind}</span>
                {a.keg_code && <code className="kcode">{a.keg_code}</code>}
                <span className="spacer" />
                <span className="small muted">{a.created_at}</span>
                <button className="small ghost" onClick={() => ackAlert(a.id)}>已知悉</button>
              </div>
              <div className="tt" style={{ marginTop: 6 }}>{a.title}</div>
              <div className="small">{a.body}</div>
            </div>
          ))}
        </div>
      )}

      <div className="row" style={{ marginBottom: 10 }}>
        <button className="ghost" disabled={sel.length === 0} onClick={() => setReserveOpen(true)}>
          预约回收（已选 {sel.length} 只空桶）
        </button>
        <button className="danger" disabled={frozenSel.length === 0} onClick={() => setConfirmOpen(true)}>
          逾期冻结桶·确认回收时间（已选 {frozenSel.length} 只）
        </button>
        <span className="spacer" />
        {frozenN > 0 && <span className="badge red">押金冻结 {frozenN} 只</span>}
        <button className="subtle" onClick={load}>刷新</button>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>选择回收</th><th>桶编号</th><th>酒款/批次</th><th>阶段</th><th>押金/账期</th>
            <th>逾期责任与举证</th><th>操作</th>
          </tr></thead>
          <tbody>
            {cycles.map(c => {
              const actionable = ['TAPPED', 'EMPTY'].includes(c.stage)
              const bucket = c.deposit_frozen ? frozenSel : sel
              return (
                <tr key={c.cycle_id} style={c.deposit_frozen ? { background: '#fff7f5' } : undefined}>
                  <td>
                    {c.stage === 'EMPTY'
                      ? <input type="checkbox" style={{ width: 'auto' }} checked={bucket.includes(c.cycle_id)}
                          onChange={() => toggle(c)} />
                      : <span className="muted small">—</span>}
                  {c.deposit_frozen && c.stage === 'EMPTY' && <div className="small" style={{ color: 'var(--danger)' }}>冻结桶</div>}
                  </td>
                  <td><code className="kcode">{c.keg_code}</code><div className="small muted">{c.capacity_liters}L</div></td>
                  <td>{c.beer}（{c.style}）<div className="small muted">{c.batch_no}</div>
                    {c.batch_status !== 'NORMAL' && <span className="badge red">批次{c.batch_status === 'SUSPENDED' ? '已停售' : '召回中'}·停止售卖</span>}
                  </td>
                  <td><Badge map={STAGE} value={c.stage} /></td>
                  <td>¥{c.deposit_amount}<div className={'small ' + (c.overdue ? '' : 'muted')} style={c.overdue ? { color: 'var(--danger)' } : undefined}>
                    账期至 {c.deposit_due_date}{c.overdue ? ' · 已逾期' : ''}
                  </div>
                    {c.release_due_date && <div className="small" style={{ color: 'var(--purple, #7c5cbf)' }}>新释放日 {c.release_due_date.slice(0, 10)}</div>}
                  </td>
                  <td>
                    <Flags items={[
                      c.deposit_frozen && '押金已冻结',
                      c.overdue && !c.deposit_frozen && '账期逾期',
                      c.bar_not_ready_count > 0 && `到店未备货×${c.bar_not_ready_count}`,
                      c.driver_fail_count > 0 && `司机到店失败×${c.driver_fail_count}`,
                      c.photo_pending > 0 && '现场照待核实',
                      c.photo_approved > 0 && '现场照已采信',
                      c.missing_scan && '司机漏扫', c.lost && '丢失'
                    ]} />
                    {c.liability_party && <div style={{ marginTop: 4 }}><Badge map={LIABILITY} value={c.liability_party} /></div>}
                  </td>
                  <td>
                    {actionable && <button className="small ghost" onClick={() => setFb(c)}>开桶反馈</button>}
                    {c.stage === 'TAPPED' &&
                      <button className="small" style={{ marginLeft: 6 }}
                        onClick={async () => {
                          try { await api('/cycles/empty/' + c.cycle_id, { method: 'POST' }); toast('已标记打空'); load() }
                          catch (e) { toast(e.message, 'err') }
                        }}>
                        标记打空
                      </button>}
                    {c.deposit_frozen && c.stage !== 'RETURNED' && c.stage !== 'WASHED' && c.stage !== 'FAILED' &&
                      <button className="small subtle" style={{ marginLeft: 6 }} onClick={() => setPhotoTarget(c)}>
                        上传打空现场照
                      </button>}
                    {['RESERVED', 'RETURNED', 'WASHED', 'FAILED'].includes(c.stage) &&
                      <span className="muted small">{c.deposit_frozen ? '已确认回收时间，等待取桶' : '回收流程中'}</span>}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {cycles.length === 0 && <Empty text="本店当前没有在店酒桶" />}
      </div>

      {fb && <FeedbackModal c={fb} onClose={() => setFb(null)} onDone={() => { setFb(null); load() }} />}
      {reserveOpen && <ReserveModal ids={sel} frozen={false} onClose={() => setReserveOpen(false)}
        onDone={() => { setReserveOpen(false); setSel([]); load() }} />}
      {confirmOpen && <ReserveModal ids={frozenSel} frozen onClose={() => setConfirmOpen(false)}
        onDone={() => { setConfirmOpen(false); setFrozenSel([]); load() }} />}
      {photoTarget && <PhotoModal c={photoTarget} onClose={() => setPhotoTarget(null)}
        onDone={() => { setPhotoTarget(null); load() }} />}
    </div>
  )
}

function FeedbackModal({ c, onClose, onDone }) {
  const [f, setF] = useState({ pour_speed: '正常', pressure: '正常', taste_off: false, taste_note: '', complaint: '', reported_by: '' })
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const toast = useToast()
  const submit = async () => {
    try {
      const d = await api('/cycles/feedback/' + c.cycle_id, { method: 'POST', body: f })
      toast(d.first_tap ? '已记录反馈，该桶首次开桶' : '反馈已同步给品控')
      if (d.warning) toast(d.warning, 'err')
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  return (
    <Modal title={`开桶反馈 · ${c.keg_code}（${c.beer}）`} onClose={onClose}>
      <div className="grid cols-2">
        <div><label>出酒速度</label>
          <select value={f.pour_speed} onChange={e => set('pour_speed', e.target.value)}>
            <option>正常</option><option>偏慢</option><option>过快</option><option>不出酒</option>
          </select></div>
        <div><label>气压</label>
          <select value={f.pressure} onChange={e => set('pressure', e.target.value)}>
            <option>正常</option><option>气压过高</option><option>气压过低</option><option>波动</option>
          </select></div>
      </div>
      <div className="checks" style={{ marginTop: 12 }}>
        <label><input type="checkbox" checked={f.taste_off} onChange={e => set('taste_off', e.target.checked)} /> 口味异常（酸/纸板味/异味等）</label>
      </div>
      {f.taste_off && <div style={{ marginTop: 6 }}><label>口味描述</label>
        <input value={f.taste_note} onChange={e => set('taste_note', e.target.value)} placeholder="如：入口发酸，后段有纸板味" /></div>}
      <div style={{ marginTop: 6 }}><label>客诉记录</label>
        <textarea rows={2} value={f.complaint} onChange={e => set('complaint', e.target.value)} placeholder="客人反馈/退酒情况" /></div>
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>提交反馈</button>
      </div>
    </Modal>
  )
}

function ReserveModal({ ids, frozen, onClose, onDone }) {
  const [date, setDate] = useState('2026-10-03')
  const [note, setNote] = useState('')
  const toast = useToast()
  const submit = async () => {
    const path = frozen ? '/cycles/confirm-return' : '/cycles/reserve'
    try {
      const d = await api(path, { method: 'POST', body: { cycle_ids: ids, pickup_date: date, note } })
      toast(frozen ? `回收时间已确认（单号 ${d.reservation_id}），等待司机上门` : `预约成功（单号 ${d.reservation_id}），等待司机上门`)
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  return (
    <Modal title={frozen ? `逾期冻结桶 · 确认回收时间 · ${ids.length} 只` : `预约空桶回收 · ${ids.length} 只`} onClose={onClose}>
      {frozen && <div className="small" style={{ color: 'var(--danger)', marginBottom: 8 }}>
        逾期冻结桶重新排期：请务必在所选日期前打空并备好空桶，新的回收时间将作为押金释放时间重新计算的依据。
      </div>}
      <label>{frozen ? '承诺备好空桶日期（司机取桶日）' : '期望取桶日期'}</label>
      <input type="date" value={date} onChange={e => setDate(e.target.value)} />
      <label>给司机的备注（停车/入口等）</label>
      <input value={note} onChange={e => setNote(e.target.value)} />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>{frozen ? '确认回收时间' : '确认预约'}</button>
      </div>
    </Modal>
  )
}

function PhotoModal({ c, onClose, onDone }) {
  const [preview, setPreview] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  const pick = async (file) => {
    if (!file) return
    try {
      const url = await fileToDataURL(file)
      setPreview(url)
    } catch (e) { toast(e.message, 'err') }
  }
  const submit = async () => {
    if (!preview) { toast('请先拍摄或选择现场照片', 'err'); return }
    setBusy(true)
    try {
      await api('/cycles/photo/' + c.cycle_id, { method: 'POST', body: { photo_data: preview, note } })
      toast('现场照片已上传，等待财务核实')
      onDone()
    } catch (e) { toast(e.message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={`已打空但未回收 · 现场举证 · ${c.keg_code}`} onClose={onClose}>
      <div className="small muted" style={{ marginBottom: 8 }}>
        押金冻结后，请拍摄能看清桶编号、桶已打空且仍在店内的现场照片；财务采信后将作为逾期责任判定依据
        （司机到店失败 + 照片采信可支持酒吧免责）。
      </div>
      <div className="row" style={{ gap: 10, alignItems: 'flex-end' }}>
        <div>
          <label>现场拍照</label>
          <input type="file" accept="image/*" capture="environment" style={{ width: 'auto' }}
            onChange={e => pick(e.target.files[0])} />
        </div>
        <div>
          <label>或从相册选择</label>
          <input type="file" accept="image/*" style={{ width: 'auto' }}
            onChange={e => pick(e.target.files[0])} />
        </div>
      </div>
      {preview && <img src={preview} alt="现场预览" style={{ maxWidth: '100%', maxHeight: 260, marginTop: 10, borderRadius: 8, border: '1px solid #ddd' }} />}
      <label style={{ marginTop: 8 }}>情况说明</label>
      <textarea rows={2} value={note} onChange={e => setNote(e.target.value)}
        placeholder="如：桶已于 9-16 打空，9-22 司机车辆故障未拉走，一直放在店内角落" />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button disabled={busy} onClick={submit}>上传举证照片</button>
      </div>
    </Modal>
  )
}

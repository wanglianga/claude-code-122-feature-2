import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { Badge, STAGE, DEPOSIT, BATCH_STATUS, RESPONSIBILITY, VISIT_RESULT, ALERT_KIND, Modal, useToast, Flags, Empty } from '../components.jsx'

export default function BarPage() {
  const [cycles, setCycles] = useState([])
  const [barId, setBarId] = useState(null)
  const [alerts, setAlerts] = useState([])
  const [fb, setFb] = useState(null)
  const [reserve, setReserve] = useState([])
  const [reserveOpen, setReserveOpen] = useState(false)
  const [proof, setProof] = useState(null)
  const [confirm, setConfirm] = useState(null)
  const toast = useToast()

  const load = () => {
    api('/bar/cycles').then(d => { setCycles(d.cycles); setBarId(d.bar_id) })
    api('/bar/alerts').then(d => setAlerts(d.alerts))
  }
  useEffect(load, [])

  const toggleReserve = (id) => setReserve(x => x.includes(id) ? x.filter(v => v !== id) : [...x, id])
  const unacked = alerts.filter(a => !a.acked)
  const ack = async (id) => {
    await api('/bar/alerts/ack/' + id, { method: 'POST' })
    toast('已确认')
    load()
  }

  return (
    <div>
      <h1>我的在店酒桶</h1>
      <div className="muted small" style={{ marginBottom: 12 }}>
        开桶后请反馈出酒速度、气压、口味异常与客诉；打空后预约回收并确认回收时间。
        押金被<b>逾期冻结</b>后可上传"已打空但未回收"现场照片申诉——司机到店失败与酒吧未备货会分别影响逾期责任。
      </div>

      {unacked.length > 0 && (
        <div className="panel" style={{ borderColor: '#e0b3aa', background: '#fdf3f1', marginBottom: 14 }}>
          <h3 style={{ marginTop: 0 }}>催还 / 责任提醒（{unacked.length}）</h3>
          {unacked.map(a => (
            <div key={a.id} className="notif" style={{ background: '#fff' }}>
              <div className="row">
                <span className="badge red">{(ALERT_KIND[a.kind] || [a.kind])[0]}</span>
                <code className="kcode">{a.keg_code}</code>
                <span className="small muted">{a.created_at}</span>
                <span className="spacer" />
                <button className="small ghost" onClick={() => ack(a.id)}>已知悉</button>
              </div>
              <div className="tt" style={{ marginTop: 4 }}>{a.title}</div>
              <div className="small">{a.body}</div>
            </div>
          ))}
        </div>
      )}

      <div className="row" style={{ marginBottom: 10 }}>
        <button className="ghost" disabled={reserve.length === 0} onClick={() => setReserveOpen(true)}>
          预约回收（已选 {reserve.length} 只空桶）
        </button>
        <span className="spacer" />
        <button className="subtle" onClick={load}>刷新</button>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>选择回收</th><th>桶编号</th><th>酒款/批次</th><th>阶段</th><th>押金/账期</th>
            <th>冻结与责任</th><th>操作</th>
          </tr></thead>
          <tbody>
            {cycles.map(c => {
              const actionable = ['TAPPED', 'EMPTY', 'RESERVED'].includes(c.stage)
              const frozen = c.deposit_status === 'FROZEN'
              return (
                <tr key={c.cycle_id} style={frozen ? { background: '#fdf6f5' } : undefined}>
                  <td>
                    {c.stage === 'EMPTY'
                      ? <input type="checkbox" style={{ width: 'auto' }} checked={reserve.includes(c.cycle_id)}
                          onChange={() => toggleReserve(c.cycle_id)} />
                      : <span className="muted small">—</span>}
                  </td>
                  <td><code className="kcode">{c.keg_code}</code><div className="small muted">{c.capacity_liters}L</div></td>
                  <td>{c.beer}（{c.style}）<div className="small muted">{c.batch_no}</div>
                    {c.batch_status !== 'NORMAL' && <span className="badge red">批次{c.batch_status === 'SUSPENDED' ? '已停售' : '召回中'}·停止售卖</span>}
                  </td>
                  <td><Badge map={STAGE} value={c.stage} /></td>
                  <td>¥{c.deposit_amount}<div className={'small ' + (c.overdue ? '' : 'muted')} style={c.overdue ? { color: 'var(--danger)' } : undefined}>
                    账期至 {c.deposit_due_date}{c.overdue ? ' · 已逾期' : ''}
                  </div>
                    {frozen && <div><Badge map={DEPOSIT} value="FROZEN" /><span className="small muted"> 于 {c.deposit_frozen_at}</span></div>}
                    {c.overdue_responsibility !== 'PENDING' && c.overdue_responsibility &&
                      <div><Badge map={RESPONSIBILITY} value={c.overdue_responsibility} /></div>}
                    {c.deposit_release_date && <div className="small" style={{ color: 'var(--ok)' }}>重算释放日 {c.deposit_release_date}</div>}
                  </td>
                  <td>
                    <Flags items={[c.overdue && '账期逾期', c.late_return && '延迟归还', c.missing_scan && '司机漏扫', c.lost && '丢失']} />
                    {c.visit_result && <div style={{ marginTop: 4 }}><Badge map={VISIT_RESULT} value={c.visit_result} /></div>}
                    {c.proof_count > 0 && <div className="small muted" style={{ marginTop: 2 }}>已上传现场照片 ×{c.proof_count}</div>}
                  </td>
                  <td>
                    {actionable && c.stage === 'TAPPED' && <button className="small ghost" onClick={() => setFb(c)}>开桶反馈</button>}
                    {c.stage === 'TAPPED' &&
                      <button className="small" style={{ marginLeft: 6 }}
                        onClick={async () => { try { await api('/cycles/empty/' + c.cycle_id, { method: 'POST' }); toast('已标记打空，可预约回收'); load() } catch (e) { toast(e.message, 'err') } }}>
                        标记打空
                      </button>}
                    {['EMPTY', 'RESERVED'].includes(c.stage) && <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                      <button className="small ghost" onClick={() => setProof(c)}>
                        上传已打空照片{c.proof_count ? `（${c.proof_count}）` : ''}
                      </button>
                      <button className="small" onClick={() => setConfirm(c)}>确认回收时间</button>
                    </div>}
                    {frozen && <div className="small" style={{ color: 'var(--danger)', marginTop: 4 }}>押金冻结中，等待责任判定</div>}
                    {['RETURNED', 'WASHED', 'FAILED'].includes(c.stage) && <span className="muted small">回收流程中</span>}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {cycles.length === 0 && <Empty text="本店当前没有在店酒桶" />}
      </div>

      {fb && <FeedbackModal c={fb} onClose={() => setFb(null)} onDone={() => { setFb(null); load() }} />}
      {reserveOpen && <ReserveModal ids={reserve} onClose={() => setReserveOpen(false)}
        onDone={() => { setReserveOpen(false); setReserve([]); load() }} />}
      {proof && <ProofModal c={proof} onClose={() => setProof(null)} onDone={() => { setProof(null); load() }} />}
      {confirm && <ConfirmPickupModal c={confirm} onClose={() => setConfirm(null)} onDone={() => { setConfirm(null); load() }} />}
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

function ReserveModal({ ids, onClose, onDone }) {
  const [date, setDate] = useState('2026-09-30')
  const [note, setNote] = useState('')
  const toast = useToast()
  const submit = async () => {
    try {
      const d = await api('/cycles/reserve', { method: 'POST', body: { cycle_ids: ids, pickup_date: date, note } })
      toast(`预约成功（单号 ${d.reservation_id}），等待司机上门`)
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  return (
    <Modal title={`预约空桶回收 · ${ids.length} 只`} onClose={onClose}>
      <label>期望取桶日期</label>
      <input type="date" value={date} onChange={e => setDate(e.target.value)} />
      <label>给司机的备注（停车/入口等）</label>
      <input value={note} onChange={e => setNote(e.target.value)} />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>确认预约</button>
      </div>
    </Modal>
  )
}

// 逾期冻结后"已打空但未回收"现场照片：本地压缩为 JPEG data URI 上传
function ProofModal({ c, onClose, onDone }) {
  const [img, setImg] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  const pick = (file) => {
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const im = new Image()
      im.onload = () => {
        const max = 960
        const scale = Math.min(1, max / Math.max(im.width, im.height))
        const cv = document.createElement('canvas')
        cv.width = Math.round(im.width * scale)
        cv.height = Math.round(im.height * scale)
        cv.getContext('2d').drawImage(im, 0, 0, cv.width, cv.height)
        setImg(cv.toDataURL('image/jpeg', 0.72))
      }
      im.src = reader.result
    }
    reader.readAsDataURL(file)
  }

  const submit = async () => {
    if (!img) { toast('请先选择现场照片', 'err'); return }
    setBusy(true)
    try {
      await api('/cycles/proofs/' + c.cycle_id, { method: 'POST', body: { photo_data: img, note } })
      toast('现场照片已上传，将作为逾期责任判定凭证')
      onDone()
    } catch (e) { toast(e.message, 'err') } finally { setBusy(false) }
  }

  return (
    <Modal title={`上传"已打空未回收"现场照片 · ${c.keg_code}`} onClose={onClose}>
      <div className="small muted" style={{ marginBottom: 8 }}>
        押金逾期冻结后，上传空桶仍在店内的照片说明情况；照片与到场记录一起用于逾期责任判定。
      </div>
      <input type="file" accept="image/*" style={{ width: 'auto' }} onChange={e => pick(e.target.files[0])} />
      {img && <img src={img} alt="现场预览" style={{ maxWidth: '100%', maxHeight: 260, marginTop: 10, borderRadius: 6, border: '1px solid #ddd0b8' }} />}
      <label style={{ marginTop: 8 }}>情况说明</label>
      <textarea rows={2} value={note} onChange={e => setNote(e.target.value)}
        placeholder="如：9/18 已打空放后厨，司机一直没来收 / 已多次电话联系" />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button disabled={busy} onClick={submit}>提交照片</button>
      </div>
    </Modal>
  )
}

// 店长确认回收时间：改约最近一张未结束预约，没有则新建
function ConfirmPickupModal({ c, onClose, onDone }) {
  const [date, setDate] = useState(c.reservation_pickup_date || '2026-09-30')
  const [note, setNote] = useState('')
  const toast = useToast()
  const submit = async () => {
    try {
      await api('/cycles/confirm-pickup', {
        method: 'POST',
        body: { cycle_id: c.cycle_id, pickup_date: date, note }
      })
      toast('回收时间已确认，平台将据此安排司机并重新计算押金释放时间')
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  return (
    <Modal title={`确认回收时间 · ${c.keg_code}`} onClose={onClose}>
      {c.visit_result === 'DRIVER_FAILED' &&
        <div className="small" style={{ color: 'var(--danger)', marginBottom: 8 }}>
          系统记录该桶预约取桶时司机未到店，请重新确认交桶时间，该记录将影响逾期责任判定。
        </div>}
      <label>可交回空桶的日期</label>
      <input type="date" value={date} onChange={e => setDate(e.target.value)} />
      <label>备注（空桶摆放位置/联系人）</label>
      <input value={note} onChange={e => setNote(e.target.value)} placeholder="如：空桶已放店门口，随时可取" />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>确认回收时间</button>
      </div>
    </Modal>
  )
}

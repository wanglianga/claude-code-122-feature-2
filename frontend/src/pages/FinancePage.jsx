import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api.js'
import { Badge, DEPOSIT, STAGE, useToast, Empty, Stat, Money, LIABILITY, PHOTO_STATUS, Modal } from '../components.jsx'

export default function FinancePage() {
  const [sp] = useSearchParams()
  const [filter, setFilter] = useState(sp.get('overdue') ? 'overdue' : '')
  const [data, setData] = useState(null)
  const [photos, setPhotos] = useState([])
  const [tab, setTab] = useState('deposits')
  const [detail, setDetail] = useState(null)
  const toast = useToast()
  const load = () => {
    const q = filter === 'overdue' ? '?overdue=1' : filter === 'frozen' ? '?frozen=1' : filter ? '?status=' + filter : ''
    api('/finance/deposits' + q).then(setData)
    api('/finance/photo-queue').then(d => setPhotos(d.photos || []))
  }
  useEffect(load, [filter])

  const scanOverdue = async () => {
    try {
      const d = await api('/overdue/scan', { method: 'POST', body: {} })
      toast(`逾期扫描完成：新冻结 ${d.frozen_count} 只桶押金，已提醒店长预约回收`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  const settle = async (c, action) => {
    let deduct = 0
    let reason = ''
    if (action === 'partial') {
      const v = prompt(`对 ${c.keg_code} 部分扣款金额（押金 ¥${c.deposit_amount}）：`)
      if (!v) return
      deduct = Number(v)
      reason = prompt('扣款原因：') || ''
    } else if (action === 'deduct') {
      reason = prompt('扣款原因：', '酒桶丢失/严重损坏，押金全额扣除') || ''
    }
    try {
      const d = await api('/finance/settle/' + c.cycle_id, { method: 'POST', body: { action, deduct_amount: deduct, reason } })
      toast(`结算完成：${d.deposit_status} 释放 ¥${d.released_amount} 扣款 ¥${d.deducted_amount}`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  const reviewPhoto = async (p, action) => {
    const note = action === 'reject' ? prompt('不采信原因：') || '照片无法确认桶已打空' : ''
    try {
      await api('/finance/photos/' + p.id, { method: 'POST', body: { action, note } })
      toast(action === 'approve' ? '照片已采信，将作为酒吧免责依据' : '照片已驳回，已通知店长')
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  if (!data) return <div className="muted">加载中…</div>
  const s = data.summary

  return (
    <div>
      <div className="row">
        <h1 style={{ margin: 0 }}>押金财务</h1>
        <span className="spacer" />
        <button className="danger" onClick={scanOverdue}>扫描逾期空桶并冻结押金</button>
        <button className={tab === 'photos' ? '' : 'ghost'} onClick={() => setTab(tab === 'photos' ? 'deposits' : 'photos')}>
          {tab === 'photos' ? '返回押金列表' : `冻结举证照片（${s.pending_photos || 0}）`}
        </button>
      </div>

      {tab === 'photos' ? (
        <PhotoQueue photos={photos} onReview={reviewPhoto} />
      ) : (<>
        <div className="grid cols-4">
          <Stat label="占用中押金" value={<Money v={s.held_amount} />} kind="warn" sub={`${s.held_count} 只桶`} />
          <Stat label="逾期已冻结押金" value={<Money v={s.frozen_amount} />} kind={s.frozen_count ? 'alert' : 'ok'}
            sub={`${s.frozen_count} 只桶 · 待责任确认`} />
          <Stat label="累计释放押金" value={<Money v={s.released_total} />} kind="ok" />
          <Stat label="累计扣款押金" value={<Money v={s.deducted_total} />} kind="alert" />
        </div>

        <div className="toolbar" style={{ marginTop: 16 }}>
          <div className="f"><label>筛选</label>
            <select value={filter} onChange={e => setFilter(e.target.value)}>
              <option value="">全部周期</option>
              <option value="frozen">仅逾期冻结（待责任确认）</option>
              <option value="overdue">仅账期逾期</option>
              <option value="HELD">押金占用中</option>
              <option value="RELEASED">已释放</option>
              <option value="DEDUCTED">已扣款</option>
              <option value="PARTIAL">部分扣款</option>
            </select></div>
        </div>

        <div className="tablewrap panel" style={{ padding: 0 }}>
          <table>
            <thead><tr>
              <th>桶编号</th><th>酒吧/城市</th><th>酒款/批次</th><th>账期</th>
              <th>押金</th><th>冻结/逾期责任</th><th>桶况与异常</th><th>状态</th><th>按桶操作</th>
            </tr></thead>
            <tbody>
              {data.deposits.map(c => (
                <tr key={c.cycle_id} style={c.deposit_frozen ? { background: '#fff7f5' } : undefined}>
                  <td><code className="kcode">{c.keg_code}</code></td>
                  <td>{c.bar}<div className="small muted">{c.city}</div></td>
                  <td className="small">{c.style} {c.batch_no}</td>
                  <td className="small">{c.deposit_due_date}
                    {c.overdue_days > 0 && c.deposit_status === 'HELD' &&
                      <div style={{ color: 'var(--danger)' }}>逾期 {c.overdue_days} 天</div>}</td>
                  <td><Money v={c.deposit_amount} /></td>
                  <td className="small">
                    {c.deposit_frozen && <span className="badge red">押金冻结中</span>}
                    {c.liability_party
                      ? <span style={{ marginLeft: 4 }}><Badge map={LIABILITY} value={c.liability_party} /></span>
                      : c.deposit_frozen && <span className="muted small"> 待责任确认</span>}
                    {c.bar_not_ready_count > 0 && <div><span className="badge red">到店未备货×{c.bar_not_ready_count}</span></div>}
                    {c.driver_fail_count > 0 && <div><span className="badge purple">司机到店失败×{c.driver_fail_count}</span></div>}
                    {c.photo_approved > 0 && <div><span className="badge green">打空照片已采信×{c.photo_approved}</span></div>}
                    {c.photo_pending > 0 && <div><span className="badge amber">照片待核实×{c.photo_pending}</span></div>}
                    {c.release_due_date && <div style={{ color: 'var(--purple,#7c5cbf)' }}>新释放日 {c.release_due_date.slice(0, 10)}</div>}
                    {c.liability_note && <div className="muted">{c.liability_note}</div>}
                  </td>
                  <td className="small">
                    {[c.late_return && '延迟归还', c.missing_scan && '司机漏扫', c.lost && '丢失', c.deformed && '变形',
                      (c.residue_level !== '无' && c.residue_level) && `残液:${c.residue_level}`,
                      (c.cap_status !== '已封盖' && c.cap_status) && c.cap_status]
                      .filter(Boolean).map((x, i) => <span key={i} className="badge red" style={{ margin: 2 }}>{x}</span>)}
                    {!c.late_return && !c.missing_scan && !c.lost && !c.deformed && c.residue_level === '无' && c.cap_status === '已封盖'
                      && <span className="muted">桶况完好</span>}
                  </td>
                  <td><Badge map={DEPOSIT} value={c.deposit_status} /><div className="small muted"><Badge map={STAGE} value={c.stage} /></div></td>
                  <td>
                    {c.deposit_status === 'HELD' && c.deposit_frozen && !c.liability_party &&
                      <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                        <button className="small" onClick={() => setDetail(c)}>逾期责任确认（按证据判定）</button>
                      </div>}
                    {c.deposit_status === 'HELD' && !c.deposit_frozen &&
                      <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                        <button className="small green" style={{ background: 'var(--ok)', borderColor: 'var(--ok)' }}
                          onClick={() => settle(c, 'release')}>释放押金</button>
                        <button className="small subtle" onClick={() => settle(c, 'partial')}>部分扣款</button>
                        <button className="small danger" onClick={() => settle(c, 'deduct')}>全额扣款</button>
                      </div>}
                    {c.deposit_status === 'HELD' && c.liability_party &&
                      <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                        <button className="small green" style={{ background: 'var(--ok)', borderColor: 'var(--ok)' }}
                          onClick={() => settle(c, 'release')}>
                          {c.release_due_date ? `释放（新释放日 ${c.release_due_date.slice(0, 10)}）` : '释放押金'}
                        </button>
                        <button className="small subtle" onClick={() => settle(c, 'partial')}>部分扣款</button>
                        <button className="small danger" onClick={() => settle(c, 'deduct')}>全额扣款</button>
                      </div>}
                    {c.deposit_status !== 'HELD' && <span className="muted small">{c.settled_at ? `已结算 ${c.settled_at}` : '已结算'}</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.deposits.length === 0 && <Empty text="无符合条件的记录" />}
        </div>
      </>)}

      {detail && <LiabilityModal c={detail} onClose={() => setDetail(null)}
        onDone={() => { setDetail(null); load() }} />}
    </div>
  )
}

function PhotoQueue({ photos, onReview }) {
  const [zoom, setZoom] = useState(null)
  return (
    <div className="panel" style={{ marginTop: 12 }}>
      <h3 style={{ marginTop: 0 }}>逾期冻结桶 · “已打空但未回收”现场举证</h3>
      <div className="small muted" style={{ marginBottom: 10 }}>
        店长在押金冻结后上传现场照片。采信后作为逾期责任判定证据：配合“司机到店失败”可支持酒吧免责；照片本身只证明桶已打空，不直接免除酒吧未及时预约的责任。
      </div>
      <div className="grid cols-2">
        {photos.map(p => (
          <div key={p.id} className="panel" style={{ margin: 0 }}>
            <div className="row">
              <code className="kcode">{p.keg_code}</code>
              <span className="muted small">{p.bar}</span>
              <span className="spacer" />
              <Badge map={PHOTO_STATUS} value={p.status} />
            </div>
            <img src={p.photo_data} alt="现场举证" onClick={() => setZoom(p)}
              style={{ width: '100%', maxHeight: 220, objectFit: 'contain', margin: '8px 0', cursor: 'zoom-in', borderRadius: 8, border: '1px solid #ddd' }} />
            <div className="small muted">{p.created_at} 上传{p.note ? '：' + p.note : ''}</div>
            {p.review_note && <div className="small">审核：{p.review_note}</div>}
            <div className="small" style={{ marginTop: 6 }}>
              {p.bar_not_ready_count > 0 && <span className="badge red" style={{ marginRight: 4 }}>到店未备货×{p.bar_not_ready_count}</span>}
              {p.driver_fail_count > 0 && <span className="badge purple">司机到店失败×{p.driver_fail_count}</span>}
            </div>
            {p.status === 'PENDING' && <div className="row end" style={{ marginTop: 8 }}>
              <button className="subtle small danger" onClick={() => onReview(p, 'reject')}>不采信</button>
              <button className="small green" style={{ background: 'var(--ok)', borderColor: 'var(--ok)' }}
                onClick={() => onReview(p, 'approve')}>采信照片</button>
            </div>}
          </div>
        ))}
      </div>
      {photos.length === 0 && <Empty text="暂无举证照片" />}
      {zoom && <Modal title={`现场照片 · ${zoom.keg_code}`} onClose={() => setZoom(null)}>
        <img src={zoom.photo_data} alt="现场举证" style={{ width: '100%' }} />
        <div className="small muted">{zoom.note}</div>
      </Modal>}
    </div>
  )
}

function LiabilityModal({ c, onClose, onDone }) {
  const [party, setParty] = useState('')
  const [note, setNote] = useState('')
  const [res, setRes] = useState(null)
  const toast = useToast()

  const submit = async () => {
    try {
      const d = await api('/finance/liability/' + c.cycle_id, { method: 'POST', body: { party, note } })
      setRes(d)
      toast(`责任已确认：${d.party}，押金新释放日 ${d.release_due_date}`)
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  const suggest = async () => {
    // 先调自动判定给用户看结论，再让其决定是否落账
    try {
      const d = await api('/finance/liability/' + c.cycle_id, { method: 'POST', body: {} })
      setRes(d)
      setParty(d.party)
      setNote(d.suggestion)
      toast(`证据判定建议：${d.suggestion}（已记录，押金新释放日 ${d.release_due_date}）`)
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }

  return (
    <Modal title={`逾期责任确认 · ${c.keg_code}（${c.bar}）`} onClose={onClose}>
      <div className="small" style={{ marginBottom: 10 }}>
        账期 {c.deposit_due_date} · 押金 ¥{c.deposit_amount} · 逾期 {c.overdue_days} 天
      </div>
      <div style={{ marginBottom: 10 }}>
        {c.bar_not_ready_count > 0 && <span className="badge red" style={{ marginRight: 6 }}>司机到店但酒吧未备货×{c.bar_not_ready_count}</span>}
        {c.driver_fail_count > 0 && <span className="badge purple" style={{ marginRight: 6 }}>司机到店失败×{c.driver_fail_count}</span>}
        {c.photo_approved > 0 && <span className="badge green" style={{ marginRight: 6 }}>打空照片已采信×{c.photo_approved}</span>}
        {c.photo_pending > 0 && <span className="badge amber" style={{ marginRight: 6 }}>照片待核实×{c.photo_pending}</span>}
      </div>
      <div className="small muted" style={{ marginBottom: 8 }}>
        判定规则：仅有“司机到店失败”且无未备货记录（或已采信打空照片）→ 酒吧无责，押金即时可释放；
        存在“到店未备货”或无免责证据 → 酒吧责任，取桶日后保留 7 天冷静期，押金释放时间据此重新计算。
      </div>
      <label>责任方（可手动覆盖系统建议）</label>
      <select value={party} onChange={e => setParty(e.target.value)}>
        <option value="">请选择（先看系统建议）</option>
        <option value="BAR">酒吧方责任（未备好空桶/延迟归还）</option>
        <option value="DRIVER">司机/物流责任（到店失败/漏扫）</option>
        <option value="NONE">酒吧无责（桶已按时打空，回收延误在物流）</option>
      </select>
      <label>责任说明</label>
      <textarea rows={2} value={note} onChange={e => setNote(e.target.value)} placeholder="判定依据/与店长沟通结论" />
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button className="ghost" onClick={suggest}>按到场与照片证据自动判定</button>
        <button onClick={submit} disabled={!party}>确认责任并重算释放时间</button>
      </div>
      {res && <div className="small" style={{ marginTop: 8, color: 'var(--ok,#2e7d32)' }}>
        已确认：{res.suggestion}；押金新释放日 {res.release_due_date}
      </div>}
    </Modal>
  )
}

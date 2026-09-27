import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { Badge, BATCH_STATUS, Modal, useToast, Empty, Money } from '../components.jsx'

export default function Batches() {
  const [list, setList] = useState([])
  const [opts, setOpts] = useState(null)
  const [detail, setDetail] = useState(null)
  const [createOpen, setCreateOpen] = useState(false)
  const toast = useToast()

  const load = () => {
    api('/batches').then(d => setList(d.batches))
    api('/options').then(setOpts)
  }
  useEffect(load, [])

  const freeze = async (id, kind) => {
    const label = kind === 'recall' ? '召回' : '暂停销售'
    const reason = prompt(`请填写${label}原因（将通知已收货酒吧、待配送订单、经销商）：`)
    if (!reason) return
    try {
      const d = await api('/batches/' + kind + '/' + id, { method: 'POST', body: { reason } })
      const n = d.notifications
      toast(`已${label}：通知酒吧 ${n.bars} 家、待配送订单 ${n.pending_orders} 个、经销商 ${n.dealers} 家`)
      load()
      setDetail(null)
    } catch (e) { toast(e.message, 'err') }
  }
  const clear = async (id) => {
    if (!confirm('确认解除该批次停售/召回，恢复销售？')) return
    await api('/batches/clear/' + id, { method: 'POST' })
    toast('批次已恢复正常')
    load(); setDetail(null)
  }
  const openDetail = (id) => api('/batches/' + id).then(setDetail)

  return (
    <div>
      <div className="row" style={{ marginBottom: 14 }}>
        <h1 style={{ margin: 0 }}>批次、停售与召回</h1>
        <span className="spacer" />
        <button className="ghost" onClick={() => setCreateOpen(true)}>新建灌装批次</button>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>批次号</th><th>酒款</th><th>灌装日期</th><th>最佳饮用期</th>
            <th>在店桶</th><th>口味投诉酒吧</th><th>状态</th><th>操作</th>
          </tr></thead>
          <tbody>
            {list.map(b => (
              <tr key={b.id}>
                <td><b>{b.batch_no}</b></td>
                <td>{b.beer} <span className="muted">{b.style}</span></td>
                <td>{b.filled_date || '—'}</td>
                <td>{b.filled_date ? `${b.best_before_days} 天` : '—'}</td>
                <td>{b.at_bar_kegs}</td>
                <td>
                  {b.taste_bars > 0
                    ? <span className="badge red">{b.taste_bars} 家 / {b.taste_complaints} 条</span>
                    : <span className="muted">无</span>}
                </td>
                <td><Badge map={BATCH_STATUS} value={b.status} /></td>
                <td>
                  <button className="small ghost" onClick={() => openDetail(b.id)}>召回库存分层</button>
                  {b.status === 'NORMAL'
                    ? <>
                        <button className="small subtle" style={{ marginLeft: 6 }} onClick={() => freeze(b.id, 'suspend')}>暂停销售</button>
                        <button className="small danger" style={{ marginLeft: 6 }} onClick={() => freeze(b.id, 'recall')}>批次召回</button>
                      </>
                    : <button className="small" style={{ marginLeft: 6 }} onClick={() => clear(b.id)}>解除停售</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.length === 0 && <Empty />}
      </div>

      {detail && <BatchDetail d={detail} onClose={() => setDetail(null)} onAction={freeze} onClear={clear} />}
      {createOpen && opts && <CreateBatch opts={opts} onClose={() => setCreateOpen(false)} onDone={() => { setCreateOpen(false); load() }} />}
    </div>
  )
}

function BatchDetail({ d, onClose, onAction, onClear }) {
  const b = d.batch
  const tierColor = ['blue', 'amber', 'purple', 'green']
  return (
    <Modal wide title={`批次召回视图 · ${b.batch_no}（${b.beer}）`} onClose={onClose}>
      <div className="row" style={{ marginBottom: 12 }}>
        <Badge map={BATCH_STATUS} value={b.status} />
        <span className="muted small">总灌装 {b.total_kegs} 只</span>
        {b.suspend_reason && <span className="badge red">停售原因：{b.suspend_reason}</span>}
        <span className="spacer" />
        {b.status === 'NORMAL'
          ? <>
              <button className="subtle small" onClick={() => onAction(b.id, 'suspend')}>暂停销售</button>
              <button className="danger small" onClick={() => onAction(b.id, 'recall')}>发起召回</button>
            </>
          : <button className="small" onClick={() => onClear(b.id)}>解除停售</button>}
      </div>

      <div className="section-title">召回库存分层（区分已售卖/已开桶/未开桶）</div>
      <div className="grid cols-3">
        {d.tiers.map((t, i) => (
          <div className="panel" key={i} style={{ margin: 0 }}>
            <span className={'badge ' + tierColor[i]}>{t.name}</span>
            <div style={{ fontSize: 24, fontWeight: 700, margin: '8px 0 2px' }}>{t.kegs} <span style={{ fontSize: 13 }} className="muted">只</span></div>
            <div className="small muted">关联押金 <Money v={t.deposit} /></div>
          </div>
        ))}
        <div className="panel" style={{ margin: 0 }}>
          <span className="badge gray">酒厂仓库未发出</span>
          <div style={{ fontSize: 24, fontWeight: 700, margin: '8px 0 2px' }}>{d.brewery_stock} <span style={{ fontSize: 13 }} className="muted">只</span></div>
          <div className="small muted">= 总灌装 − 在途/在店 − 待配送</div>
        </div>
      </div>
      <div className="panel" style={{ margin: '12px 0 0' }}>
        <span className="badge red">待配送订单（停售后冻结，避免继续铺货）</span>
        <b style={{ marginLeft: 8 }}>{d.pending_orders.kegs} 只</b>
        <table style={{ marginTop: 8 }}>
          <thead><tr><th>订单</th><th>酒吧</th><th>经销商</th><th>数量</th><th>节日铺货</th><th>状态</th></tr></thead>
          <tbody>
            {d.pending_orders.items.map((p, i) => (
              <tr key={i}><td>{p.order_no}</td><td>{p.bar}</td><td>{p.dealer}</td><td>{p.qty}</td>
                <td>{p.is_festival ? <span className="badge purple">节日临时铺货</span> : '—'}</td>
                <td><span className="badge amber">{p.status === 'PLANNED' ? '待配送' : p.status}</span></td></tr>
            ))}
            {d.pending_orders.items.length === 0 && <tr><td colSpan={6}><Empty text="无待配送订单" /></td></tr>}
          </tbody>
        </table>
      </div>

      <div className="section-title">已收货酒吧（未开桶不得继续售卖）</div>
      <div className="tablewrap" style={{ maxHeight: 200 }}>
        <table>
          <thead><tr><th>城市</th><th>酒吧</th><th>未开桶</th><th>已开桶</th><th>合计</th></tr></thead>
          <tbody>
            {d.bars.map(x => (
              <tr key={x.bar_id}><td>{x.city}</td><td>{x.bar}</td>
                <td>{x.untapped > 0 ? <span className="badge blue">{x.untapped}</span> : 0}</td>
                <td>{x.tapped > 0 ? <span className="badge amber">{x.tapped}</span> : 0}</td>
                <td>{x.total}</td></tr>
            ))}
            {d.bars.length === 0 && <tr><td colSpan={5}><Empty text="暂无在店库存" /></td></tr>}
          </tbody>
        </table>
      </div>

      <div className="section-title">开桶反馈与客诉</div>
      <div className="tablewrap" style={{ maxHeight: 220 }}>
        <table>
          <thead><tr><th>时间</th><th>酒吧</th><th>出酒</th><th>气压</th><th>口味</th><th>客诉</th></tr></thead>
          <tbody>
            {d.feedbacks.map(f => (
              <tr key={f.id}>
                <td className="small muted">{f.at}</td><td>{f.bar}</td>
                <td>{f.pour_speed || '—'}</td><td>{f.pressure || '—'}</td>
                <td>{f.taste_off ? <span className="badge red">异常：{f.taste_note}</span> : '正常'}</td>
                <td className="small">{f.complaint || '—'}</td>
              </tr>
            ))}
            {d.feedbacks.length === 0 && <tr><td colSpan={6}><Empty text="暂无反馈" /></td></tr>}
          </tbody>
        </table>
      </div>
    </Modal>
  )
}

function CreateBatch({ opts, onClose, onDone }) {
  const [f, setF] = useState({ beer_id: '', batch_no: '', brewed_date: '2026-09-25', filled_date: '2026-09-26', best_before_days: 30, total_kegs: 10 })
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const toast = useToast()
  const submit = async () => {
    try {
      await api('/batches', { method: 'POST', body: { ...f, beer_id: Number(f.beer_id), best_before_days: Number(f.best_before_days), total_kegs: Number(f.total_kegs) } })
      toast('批次已创建')
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }
  return (
    <Modal title="新建灌装批次" onClose={onClose}>
      <div className="grid cols-2">
        <div><label>酒款</label>
          <select value={f.beer_id} onChange={e => set('beer_id', e.target.value)}>
            <option value="">请选择</option>
            {opts.beers.map(b => <option key={b.id} value={b.id}>{b.name}（{b.style}）</option>)}
          </select></div>
        <div><label>批次号</label><input value={f.batch_no} placeholder="BT2026-XX-01" onChange={e => set('batch_no', e.target.value)} /></div>
        <div><label>酿造日期</label><input type="date" value={f.brewed_date} onChange={e => set('brewed_date', e.target.value)} /></div>
        <div><label>灌装日期</label><input type="date" value={f.filled_date} onChange={e => set('filled_date', e.target.value)} /></div>
        <div><label>最佳饮用期（天）</label><input type="number" value={f.best_before_days} onChange={e => set('best_before_days', e.target.value)} /></div>
        <div><label>灌装桶数</label><input type="number" value={f.total_kegs} onChange={e => set('total_kegs', e.target.value)} /></div>
      </div>
      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>创建</button>
      </div>
    </Modal>
  )
}

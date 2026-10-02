/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import { getRiskUserDetail } from '../api'
import { RISK_SIGNAL_KEYS, RISK_SIGNAL_LABELS, type RiskWindow } from '../types'

const EVIDENCE_LABELS: Record<string, string> = {
  requests: 'Requests',
  distinct_models: 'Multiple models',
  distinct_ips: 'Distinct IPs',
  repeated_inputs: 'Repeated input',
  regular_intervals: 'Regular timing',
  post_402_requests: 'Requests after 402',
  non_stream_requests: 'Non-stream requests',
  night_requests: 'Night activity',
  post_channel_unavailable_requests: 'Requests after channel unavailable',
  blocked_liveness_requests: 'Blocked liveness probes',
}

interface RiskDetailDialogProps {
  userId: number | null
  window: RiskWindow
  onClose: () => void
}

export function RiskDetailDialog(props: RiskDetailDialogProps) {
  const { t } = useTranslation()
  const details = useQuery({
    queryKey: ['risk-user', props.userId, props.window],
    queryFn: () => {
      if (props.userId === null) throw new Error('No risk user selected')
      return getRiskUserDetail(props.userId, props.window)
    },
    enabled: props.userId !== null,
  })
  const data = details.data

  return (
    <Dialog open={props.userId !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent className='relayfor-risk-dialog sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('Risk details')}{details.data ? ` · ${details.data.username}` : ''}</DialogTitle>
          <DialogDescription>{t('Scores describe request patterns. Review evidence before taking account action.')}</DialogDescription>
        </DialogHeader>
        {details.isLoading && <p>{t('Loading...')}</p>}
        {!details.isLoading && details.isError && <p role='alert'>{t('Unable to load risk details')}</p>}
        {!details.isLoading && !details.isError && data ? (
          <div className='relayfor-risk-detail-content'>
            <div className='relayfor-risk-detail-summary'>
              <span>{t('Risk score')} <strong>{Math.round(data.score)}</strong></span>
              <span>{t('Requests')} <strong>{data.request_count}</strong></span>
              <span>{t('Distinct IPs')} <strong>{data.ip_count}</strong></span>
            </div>
            <h3>{t('Signal breakdown')}</h3>
            <div className='relayfor-risk-detail-signals'>
              {RISK_SIGNAL_KEYS.map((key) => (
                <div key={key} className='relayfor-risk-detail-signal'>
                  <div><span>{t(RISK_SIGNAL_LABELS[key])}</span><strong>{Math.round(data.signals[key] ?? 0)}</strong></div>
                  <progress value={data.signals[key] ?? 0} max={100} aria-label={t(RISK_SIGNAL_LABELS[key])} />
                </div>
              ))}
            </div>
            <h3>{t('Aggregate evidence')}</h3>
            <dl className='relayfor-risk-evidence'>
              {Object.entries(data.evidence ?? {}).map(([key, value]) => (
                <div key={key}><dt>{t(EVIDENCE_LABELS[key] ?? key)}</dt><dd>{value == null ? '—' : String(value)}</dd></div>
              ))}
            </dl>
            <p className='relayfor-risk-privacy-note'>{t('Only aggregate counts are retained. Request contents and raw IP addresses are not shown.')}</p>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

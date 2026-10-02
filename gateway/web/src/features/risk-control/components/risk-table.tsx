/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { Check, Eye, Info, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { formatRiskTimestamp } from '../lib/format'
import {
  RISK_SIGNAL_KEYS,
  RISK_SIGNAL_LABELS,
  type RiskStatus,
  type RiskUser,
} from '../types'

interface RiskTableProps {
  rows: RiskUser[]
  loading: boolean
  updatingUserId: number | null
  onDetails: (userId: number) => void
  onStatus: (userId: number, status: RiskStatus) => void
}

export function RiskTable(props: RiskTableProps) {
  const { t, i18n } = useTranslation()

  return (
    <div className='relayfor-risk-table-scroll'>
      <Table withContainer={false} className='relayfor-risk-table'>
        <TableHeader>
          <TableRow>
            <TableHead>{t('ID')}</TableHead>
            <TableHead>{t('Username')}</TableHead>
            <TableHead>{t('Risk score')}</TableHead>
            <TableHead>{t('Risk level')}</TableHead>
            {RISK_SIGNAL_KEYS.map((key) => (
              <TableHead key={key}>{t(RISK_SIGNAL_LABELS[key])}</TableHead>
            ))}
            <TableHead>{t('Last request')}</TableHead>
            <TableHead>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.loading ? (
            <TableRow>
              <TableCell colSpan={RISK_SIGNAL_KEYS.length + 6} className='relayfor-risk-empty'>
                {t('Loading...')}
              </TableCell>
            </TableRow>
          ) : null}
          {!props.loading && props.rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={RISK_SIGNAL_KEYS.length + 6} className='relayfor-risk-empty'>
                {t('No risk records yet')}
              </TableCell>
            </TableRow>
          ) : null}
          {!props.loading && props.rows.length > 0 && (
            props.rows.map((row) => (
              <TableRow key={row.user_id}>
                <TableCell>{row.user_id}</TableCell>
                <TableCell>
                  <strong className='relayfor-risk-username'>{row.username}</strong>
                  <span className={`relayfor-risk-status relayfor-risk-status-${row.status}`}>
                    {t(`risk.status.${row.status}`)}
                  </span>
                </TableCell>
                <TableCell>
                  <span className='relayfor-risk-score' aria-label={`${t('Risk score')}: ${row.score}`}>
                    {Math.round(row.score)}
                  </span>
                </TableCell>
                <TableCell>
                  <span className={`relayfor-risk-level relayfor-risk-level-${row.level}`}>
                    {t(`risk.level.${row.level}`)}
                  </span>
                </TableCell>
                {RISK_SIGNAL_KEYS.map((key) => (
                  <TableCell key={key}>
                    <span className='relayfor-risk-signal' title={t(RISK_SIGNAL_LABELS[key])}>
                      {Math.round(row.signals[key] ?? 0)}
                    </span>
                  </TableCell>
                ))}
                <TableCell className='relayfor-risk-date'>
                  {formatRiskTimestamp(row.last_request_at, i18n.resolvedLanguage || i18n.language)}
                </TableCell>
                <TableCell>
                  <div className='relayfor-risk-actions'>
                    <Button variant='ghost' size='icon-sm' title={t('View risk details')} aria-label={t('View risk details')} onClick={() => props.onDetails(row.user_id)}>
                      <Info aria-hidden='true' />
                    </Button>
                    <Button variant='ghost' size='icon-sm' title={t('Watch account')} aria-label={t('Watch account')} disabled={props.updatingUserId === row.user_id || row.status === 'watching'} onClick={() => props.onStatus(row.user_id, 'watching')}>
                      <Eye aria-hidden='true' />
                    </Button>
                    <Button variant='ghost' size='icon-sm' title={t('Confirm risk')} aria-label={t('Confirm risk')} disabled={props.updatingUserId === row.user_id || row.status === 'confirmed'} onClick={() => props.onStatus(row.user_id, 'confirmed')}>
                      <Check aria-hidden='true' />
                    </Button>
                    <Button variant='ghost' size='icon-sm' title={t('Dismiss risk')} aria-label={t('Dismiss risk')} disabled={props.updatingUserId === row.user_id || row.status === 'dismissed'} onClick={() => props.onStatus(row.user_id, 'dismissed')}>
                      <X aria-hidden='true' />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}

/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, RefreshCw, Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { handleServerError } from '@/lib/handle-server-error'

import { listRiskUsers, updateRiskUserStatus } from './api'
import { RiskDetailDialog } from './components/risk-detail-dialog'
import { RiskTable } from './components/risk-table'
import type { RiskStatus, RiskWindow } from './types'

import './risk-control.css'

type RiskTab = 'alerts' | 'liveness'

export function RiskControlPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<RiskTab>('liveness')
  const [window, setWindow] = useState<RiskWindow>('7d')
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState('')
  const [username, setUsername] = useState('')
  const [level, setLevel] = useState('all')
  const [status, setStatus] = useState('all')
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [updatingUserId, setUpdatingUserId] = useState<number | null>(null)

  const selectedLevel = level === 'all' ? '' : level
  const filters = {
    window,
    page,
    page_size: 20,
    username,
    level: tab === 'alerts' ? 'attention' : selectedLevel,
    status: status === 'all' ? '' : status,
  }

  const riskUsers = useQuery({
    queryKey: ['risk-users', filters],
    queryFn: () => listRiskUsers(filters),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })

  const updateStatus = useMutation({
    mutationFn: (target: { userId: number; status: RiskStatus }) =>
      updateRiskUserStatus(target.userId, target.status),
    onMutate: (target) => setUpdatingUserId(target.userId),
    onSuccess: () => {
      toast.success(t('Risk review updated'))
      void queryClient.invalidateQueries({ queryKey: ['risk-users'] })
      void queryClient.invalidateQueries({ queryKey: ['risk-user'] })
    },
    onError: (error) => handleServerError(error),
    onSettled: () => setUpdatingUserId(null),
  })

  const total = riskUsers.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / filters.page_size))

  return (
    <Main className='relayfor-risk-page'>
      <div className='relayfor-risk-scroll'>
        <Tabs
          value={tab}
          onValueChange={(value) => {
            setTab(value === 'alerts' ? 'alerts' : 'liveness')
            setPage(1)
          }}
        >
          <TabsList variant='line' className='relayfor-risk-tabs'>
            <TabsTrigger value='alerts'>{t('Risk alerts')}</TabsTrigger>
            <TabsTrigger value='liveness'>{t('Anti-liveness')}</TabsTrigger>
          </TabsList>
        </Tabs>

        <header className='relayfor-risk-heading'>
          <div>
            <h1>{tab === 'alerts' ? t('High risk accounts') : t('Suspicious liveness accounts')}</h1>
            <p>{t('Identify suspicious probes from request behavior without storing request content.')}</p>
          </div>
          <div className='relayfor-risk-heading-actions'>
            <span>{t('Total')}: {total}</span>
            <Button
              variant='outline'
              size='icon'
              aria-label={t('Refresh risk scores')}
              title={t('Refresh risk scores')}
              onClick={() => void riskUsers.refetch()}
              disabled={riskUsers.isFetching}
            >
              <RefreshCw aria-hidden='true' />
            </Button>
          </div>
        </header>

        <section className='relayfor-risk-panel' aria-labelledby='risk-list-title'>
          <div className='relayfor-risk-panel-title'>
            <h2 id='risk-list-title'>{t('Suspicious account list')}</h2>
            <p>{t('Scores are indicators for human review, not automatic account penalties.')}</p>
          </div>

          <div className='relayfor-risk-toolbar'>
            <form
              className='relayfor-risk-search'
              onSubmit={(event) => {
                event.preventDefault()
                setUsername(searchInput.trim())
                setPage(1)
              }}
            >
              <Input
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
                placeholder={t('Search username')}
                aria-label={t('Search username')}
              />
              <Button type='submit'><Search aria-hidden='true' />{t('Search')}</Button>
            </form>

            <Select
              value={window}
              onValueChange={(value) => {
                setWindow(value === '3d' ? '3d' : '7d')
                setPage(1)
              }}
            >
              <SelectTrigger aria-label={t('Scoring window')} className='relayfor-risk-select'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='3d'>{t('Last 3 days')}</SelectItem>
                <SelectItem value='7d'>{t('Last 7 days')}</SelectItem>
              </SelectContent>
            </Select>

            {tab === 'liveness' && (
              <Select
                value={level}
                onValueChange={(value) => {
                  setLevel(typeof value === 'string' ? value : 'all')
                  setPage(1)
                }}
              >
                <SelectTrigger aria-label={t('Risk level filter')} className='relayfor-risk-select'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>{t('All levels')}</SelectItem>
                  <SelectItem value='low'>{t('risk.level.low')}</SelectItem>
                  <SelectItem value='medium'>{t('risk.level.medium')}</SelectItem>
                  <SelectItem value='high'>{t('risk.level.high')}</SelectItem>
                  <SelectItem value='critical'>{t('risk.level.critical')}</SelectItem>
                </SelectContent>
              </Select>
            )}

            <Select
              value={status}
              onValueChange={(value) => {
                setStatus(typeof value === 'string' ? value : 'all')
                setPage(1)
              }}
            >
              <SelectTrigger aria-label={t('Review status filter')} className='relayfor-risk-select'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('All review statuses')}</SelectItem>
                <SelectItem value='unreviewed'>{t('risk.status.unreviewed')}</SelectItem>
                <SelectItem value='watching'>{t('risk.status.watching')}</SelectItem>
                <SelectItem value='confirmed'>{t('risk.status.confirmed')}</SelectItem>
                <SelectItem value='dismissed'>{t('risk.status.dismissed')}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {riskUsers.isError ? (
            <div className='relayfor-risk-error' role='alert'>
              <p>{t('Unable to load risk records')}</p>
              <Button variant='outline' onClick={() => void riskUsers.refetch()}>{t('Retry')}</Button>
            </div>
          ) : (
            <RiskTable
              rows={riskUsers.data?.items ?? []}
              loading={riskUsers.isLoading}
              updatingUserId={updatingUserId}
              onDetails={setSelectedUserId}
              onStatus={(userId, nextStatus) => updateStatus.mutate({ userId, status: nextStatus })}
            />
          )}

          <div className='relayfor-risk-pagination'>
            <span>{t('Total')}: {total} · {page}/{pageCount}</span>
            <div>
              <Button variant='outline' size='icon-sm' aria-label={t('Previous page')} disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>
                <ChevronLeft aria-hidden='true' />
              </Button>
              <Button variant='outline' size='icon-sm' aria-label={t('Next page')} disabled={page >= pageCount} onClick={() => setPage((current) => current + 1)}>
                <ChevronRight aria-hidden='true' />
              </Button>
            </div>
          </div>
        </section>
      </div>

      <RiskDetailDialog userId={selectedUserId} window={window} onClose={() => setSelectedUserId(null)} />
    </Main>
  )
}

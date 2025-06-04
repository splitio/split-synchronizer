import './App.css'
import { Button, Layout, Text, ButtonVariation, TableV2, SlidingPane, Tag } from '@harness/uicore'
import { useCallback, useState, useEffect } from 'react'
import type { SlidingPaneState } from '@harness/uicore'
import moment from 'moment'
import { Intent } from '@blueprintjs/core'

interface FeatureFlag {
  name: string
  active: boolean
  killed: boolean
  defaultTreatment: string
  treatments: string[]
  flagSets: string[]
  cn: string
  changeNumber: number
}

interface DashboardStats {
  featureFlags: FeatureFlag[]
  backendTotalRequests: number
  requestsOk: number
  requestsErrored: number
  backendRequestsOk: number
  backendRequestsErrored: number
  sdksTotalRequests: number
  loggedErrors: number
  loggedMessages: string[]
  uptime: number
}

function App() {
  const [paneState, setPaneState] = useState<SlidingPaneState>('closed')
  const [selectedFlag, setSelectedFlag] = useState<FeatureFlag | null>(null)
  const [flags, setFlags] = useState<FeatureFlag[]>([])
  const [searchTerm, setSearchTerm] = useState('')
  
  const filteredFlags = flags.filter(flag => 
    flag.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    flag.treatments.some(t => t.toLowerCase().includes(searchTerm.toLowerCase()))
  )

  useEffect(() => {
    fetch('http://localhost:3010/admin/dashboard/stats')
      .then(response => response.json())
      .then((data: DashboardStats) => {
        setFlags(data.featureFlags || [])
      })
      .catch(error => console.error('Error fetching splits:', error))
  }, [])

  const formatDate = useCallback((dateStr: string) => {
    return moment.utc(dateStr, 'ddd MMM DD HH:mm:ss UTC YYYY')
      .local()
      .format('MM/DD/YY HH:mm:ss')
  }, [])

  const handleEdit = useCallback((flag: FeatureFlag) => {
    setSelectedFlag(flag)
    setPaneState('open')
  }, [])

  const handleKill = useCallback((flag: FeatureFlag) => {
    console.log('Kill:', flag)
  }, [])

  return (
    <Layout.Vertical spacing="large" style={{ alignItems: 'center', textAlign: 'center' }}>
      <Text>Feature Flags</Text>
      <Layout.Horizontal spacing="small" style={{ width: '100%', justifyContent: 'flex-end', padding: '0 16px' }}>
        <input
          type="text"
          placeholder="Search feature flags..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          style={{
            padding: '8px 12px',
            border: '1px solid #ccc',
            borderRadius: '4px',
            width: '300px',
            fontSize: '14px'
          }}
        />
      </Layout.Horizontal>
      <TableV2
        columns={[
          {
            Header: 'Feature Flag Name',
            accessor: 'name',
            id: 'name',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text style={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                maxWidth: '150px'
              }}>
                {row.original.name}
              </Text>
            )
          },
          {
            Header: 'Status',
            accessor: 'active',
            id: 'active',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text style={{ color: row.original.active ? '#42ab45' : undefined }}>
                {row.original.active ? 'Active' : 'Inactive'}
              </Text>
            )
          },
          {
            Header: 'Killed',
            accessor: 'killed',
            id: 'killed',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text>{row.original.killed ? 'Yes' : 'No'}</Text>
            )
          },
          {
            Header: 'Treatments',
            accessor: 'treatments',
            id: 'treatments',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Layout.Horizontal spacing="xsmall" style={{ flexWrap: 'wrap', gap: '4px' }}>
                {row.original.treatments.map(treatment => (
                  <Tag
                    key={treatment}
                    children={treatment}
                    intent={treatment === row.original.defaultTreatment ? Intent.PRIMARY : Intent.NONE}
                  />
                ))}
              </Layout.Horizontal>
            )
          },
          {
            Header: 'Flag Sets',
            accessor: 'flagSets',
            id: 'flagSets',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text>{row.original.flagSets.join(', ')}</Text>
            )
          },
          {
            Header: 'Last Modified',
            accessor: 'cn',
            id: 'cn',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text>{formatDate(row.original.cn)}</Text>
            )
          },
          {
            Header: 'Actions',
            id: 'actions',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Layout.Horizontal spacing="small">
                <Button
                  text="Edit"
                  onClick={() => handleEdit(row.original)}
                  minimal
                  small
                  variation={ButtonVariation.PRIMARY}
                />
                <Button
                  text="Kill"
                  onClick={() => handleKill(row.original)}
                  minimal
                  small
                  intent="danger"
                  variation={ButtonVariation.PRIMARY} 
                />
              </Layout.Horizontal>
            )
          }
        ]}
        hideHeaders={false}
        data={filteredFlags}
      />
      <SlidingPane
        title={`Edit Feature Flag: ${selectedFlag?.name || ''}`}
        state={paneState}
        onStateChange={setPaneState}
        width="400px"
      >
        {selectedFlag && (
          <Layout.Vertical spacing="large" style={{ padding: '16px' }}>
            <Text>Name: {selectedFlag.name}</Text>
            <Text>Status: {selectedFlag.active ? 'Active' : 'Inactive'}</Text>
            <Text>Default Treatment: {selectedFlag.defaultTreatment}</Text>
            <Text>Killed: {selectedFlag.killed ? 'Yes' : 'No'}</Text>
            <Layout.Horizontal spacing="xsmall" style={{ marginTop: '8px' }}>
              <Text>Treatments:</Text>
              <Layout.Horizontal spacing="xsmall" style={{ flexWrap: 'wrap', gap: '4px' }}>
                {selectedFlag.treatments.map(treatment => (
                  <Tag
                    key={treatment}
                    children={treatment}
                    style={{
                      backgroundColor: treatment === selectedFlag.defaultTreatment ? '#0072C6' : '#E0E0E0',
                      color: treatment === selectedFlag.defaultTreatment ? 'white' : '#333',
                      padding: '2px 8px',
                      borderRadius: '4px',
                      fontSize: '12px'
                    }}
                  />
                ))}
              </Layout.Horizontal>
            </Layout.Horizontal>
            <Text>Flag Sets: {selectedFlag.flagSets.join(', ')}</Text>
            <Text>Last Modified: {formatDate(selectedFlag.cn)}</Text>
            <Text>Change Number: {selectedFlag.changeNumber}</Text>
          </Layout.Vertical>
        )}
      </SlidingPane>
    </Layout.Vertical>
  )
}

export default App

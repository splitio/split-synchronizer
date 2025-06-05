import './App.css'
import { Button, Layout, Text, ButtonVariation, TableV2, SlidingPane, Tag, ModalDialog } from '@harness/uicore'
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
  isOverridden?: boolean
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
  const [editDialogOpen, setEditDialogOpen] = useState<boolean>(false)
  const [flagToEdit, setFlagToEdit] = useState<FeatureFlag | null>(null)
  
  const filteredFlags = flags.filter(flag => 
    flag.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    flag.treatments.some(t => t.toLowerCase().includes(searchTerm.toLowerCase()))
  )

  const fetchAndMergeFlags = useCallback(async () => {
    try {
      // Fetch both endpoints in parallel
      const [statsResponse, overridesResponse] = await Promise.all([
        fetch('http://localhost:3010/admin/dashboard/stats'),
        fetch('http://localhost:3010/admin/overrides/ff')
      ])

      const statsData: DashboardStats = await statsResponse.json()
      const overridesData = await overridesResponse.json()

      // Create a map of overridden flags
      const overridesMap = new Map(
        Object.entries(overridesData || {}).map(([name, flag]) => [name, flag as FeatureFlag])
      )

      // Merge the flags
      const mergedFlags = statsData.featureFlags
        .map(flag => {
          const override = overridesMap.get(flag.name)
          if (override) {
            return {
              ...flag,
              killed: override.killed,
              defaultTreatment: override.defaultTreatment,
              isOverridden: true
            }
          }
          return { ...flag, isOverridden: false }
        })
        .sort((a, b) => a.name.localeCompare(b.name))

      setFlags(mergedFlags)
    } catch (error) {
      console.error('Error fetching data:', error)
    }
  }, [])

  useEffect(() => {
    fetchAndMergeFlags()
  }, [fetchAndMergeFlags])

  const formatDate = useCallback((dateStr: string) => {
    return moment.utc(dateStr, 'ddd MMM DD HH:mm:ss UTC YYYY')
      .local()
      .format('MM/DD/YY HH:mm:ss')
  }, [])

  const handleEdit = useCallback((flag: FeatureFlag) => {
    setSelectedFlag(flag)
    setPaneState('open')
  }, [])

  const handleKillRestoreClick = useCallback((flag: FeatureFlag) => {
    setFlagToEdit(flag)
    setEditDialogOpen(true)
  }, [])

  const handleDeleteOverride = useCallback(async (flag: FeatureFlag) => {
    try {
      const response = await fetch(`http://localhost:3010/admin/overrides/ff/${flag.name}`, {
        method: 'DELETE'
      })

      if (!response.ok) {
        throw new Error('Failed to delete override')
      }

      await fetchAndMergeFlags()
    } catch (error) {
      console.error('Error deleting override:', error)
    }
  }, [fetchAndMergeFlags])

  const handleEditConfirm = useCallback(async () => {
    if (!flagToEdit) return

    try {
      const response = await fetch(`http://localhost:3010/admin/overrides/ff/${flagToEdit.name}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          killed: !flagToEdit.killed,
        })
      })

      if (!response.ok) {
        throw new Error('Failed to kill feature flag')
      }

      // Refresh the data
      await fetchAndMergeFlags()
    } catch (error) {
      console.error('Error updating feature flag:', error)
    } finally {
      setEditDialogOpen(false)
      setFlagToEdit(null)
    }
  }, [flagToEdit, fetchAndMergeFlags])

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
          },          {
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
            Header: 'Override',
            accessor: 'isOverridden',
            id: 'isOverridden',
            Cell: ({ row }: { row: { original: FeatureFlag } }) => (
              <Text>{row.original.isOverridden ? 'Yes' : '-'}</Text>
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
            width: 350,
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
                  text={row.original.killed ? 'Restore' : 'Kill'}
                  onClick={() => handleKillRestoreClick(row.original)}
                  minimal
                  small
                  intent={row.original.killed ? 'success' : 'danger'}
                  variation={ButtonVariation.PRIMARY}
                  style={{ width: '86px' }}
                />
                {row.original.isOverridden && (
                  <Button
                    text="Discard override"
                    onClick={() => handleDeleteOverride(row.original)}
                    minimal
                    small
                    intent="warning"
                    variation={ButtonVariation.PRIMARY} 
                  />
                )}
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

      {/* @ts-expect-error ModalDialog type mismatch */}
      <ModalDialog
        isOpen={editDialogOpen}
        onClose={() => setEditDialogOpen(false)}
        title={flagToEdit?.killed ? 'Restore Feature Flag' : 'Kill Feature Flag'}
        footer={
          <Layout.Horizontal spacing="small">
          <Button
            variation={ButtonVariation.PRIMARY}
            intent={flagToEdit?.killed ? 'success' : 'danger'}
            text="Yes"
            minimal
            small
            onClick={handleEditConfirm}
          />
          <Button
            variation={ButtonVariation.SECONDARY}
            text="Cancel"
            minimal
            small
            onClick={() => setEditDialogOpen(false)}
          />
        </Layout.Horizontal>
        }
      >
          <Text>
            Are you sure you want to {flagToEdit?.killed ? 'restore' : 'kill'} the feature flag "{flagToEdit?.name}"?
          </Text>
      </ModalDialog>
    </Layout.Vertical>
  )
}

export default App

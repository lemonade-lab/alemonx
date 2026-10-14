export type GitSettingsEditor = 'identity' | 'add' | 'edit'

export type GitSettingsDialogProps = {
  editor: GitSettingsEditor | null
  busy: boolean
  error: string
  name: string
  email: string
  url: string
  onNameChange: (value: string) => void
  onEmailChange: (value: string) => void
  onURLChange: (value: string) => void
  onClose: () => void
  onSubmit: () => void
}

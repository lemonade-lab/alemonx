import '../../src/styles.css'
import '@testone/testone.scss'
import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import Timer from '@testone/pages/modals/ModalCommandTomer'

export function TimerFixture() {
  const [values, setValues] = useState({ time: 13, loop: true })
  return (
    <Timer
      open
      values={values}
      onChange={setValues}
      commands={[]}
      onCancel={() => {}}
      onConfirm={() => {}}
    />
  )
}

createRoot(document.getElementById('testone')!).render(<TimerFixture />)

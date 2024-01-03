import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Link } from 'react-router-dom'

const Lobbies = () => {
  const [lobbies, setLobbies] = useState([])
  const navigate = useNavigate()

  useEffect(() => {
    fetch('/api/lobbies', {
      method: 'GET',
    })
      .then((res) => {
        if (res.status === 200) {
          return res.json()
        }

        if ( res.status === 401) {
          console.log('unauthorized')
          navigate('/login')
        }
      
    })
      .then((response) => {
        setLobbies(response)
      })
  }, [])

  const joinMatch = () => {
    console.log('joining match')
  }

  let rows = <tr><td colSpan={3}>No lobbies available</td></tr>

  if (lobbies && lobbies.length > 0) {
    rows = lobbies.map((lobby) => (
      <tr key={lobby.Id}>
        <td>{lobby.Name}</td>
        <td>{`${lobby.PlayerIds.length}/2`}</td>
        <td>
          {lobby.PlayerIds.length < 2 && (
            <button type="button" className='secondary' onClick={joinMatch}>
              Join match
            </button>
          )}
        </td>
      </tr>
    ))
  }
  return (
    <div>
      <h1>Find or Create Lobby</h1>

      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>players</th>
            <th />
          </tr>
        </thead>
        <tbody>
         {rows}
        </tbody>
      </table>

      <div>
        <Link to={`/lobbies/create`}>Create Lobby</Link>
      </div>
    </div>
  )
}

export default Lobbies

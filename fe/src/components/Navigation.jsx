import React from 'react'
import { useNavigate } from 'react-router-dom'
import { useContext } from 'react'
import { GlobalDataContext } from '../GlobalDataContext'

const navigation = () => {  
  const { globalData, setGlobalData } = useContext(GlobalDataContext)
  const navigate = useNavigate()

  const handleLogout = (e) => {
    console.log(e)
    e.preventDefault()
    fetch('/api/logout')
      .then((res) => {
        if (res.status === 200) {
          console.log('logout')
          setGlobalData(null)
          navigate('/login')
        }
      })
      .catch((err) => {
        console.log(err)
      })
  }

 return( <header>
    <nav className='row'>
      <a href="/">
        <img src="https://placehold.co/600x200" alt="Company logo" id="logo" />
      </a>
      <div className='flex-space'/>
      {
        globalData && 
        <ul className='row'>
         <li> Hello,{globalData.username}</li>
        <li><button  className='button secondary' onClick={handleLogout}>Log out</button></li>
        </ul>
      }
    </nav>
  </header>)
}

export default navigation
